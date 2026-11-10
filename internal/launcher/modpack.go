package launcher

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
)

type ModpackInfo struct {
	Source                   ModpackSource
	Name, Version, Minecraft string
	Loader, LoaderVersion    string
	Downloads, Overrides     int
}

type PreparedModpack struct {
	info     ModpackInfo
	filename string
	sum      [sha256.Size]byte
}

func (p PreparedModpack) Info() ModpackInfo { return p.info }

type modpackDownload struct {
	path  string
	asset modpackAsset
}

type modpackOverride struct {
	path string
	file *zip.File
}

type modpackPlan struct {
	info      ModpackInfo
	downloads []modpackDownload
	curse     []curseReference
	overrides []modpackOverride
}

type mrpackManifest struct {
	FormatVersion         int
	Game, Name, VersionID string
	Dependencies          map[string]string
	Files                 []struct {
		Path      string
		FileSize  *int64
		Hashes    struct{ SHA512 string }
		Downloads []string
		Env       struct{ Client string }
	}
}

type curseManifest struct {
	ManifestType, Name, Version, Overrides string
	ManifestVersion                        int
	Minecraft                              struct {
		Version    string
		ModLoaders []struct {
			ID      string
			Primary bool
		}
	}
	Files []curseReference
}

func (g Game) PrepareOnlineModpack(ctx context.Context, r ModpackRelease) (PreparedModpack, error) {
	if r.Unavailable != "" {
		return PreparedModpack{}, errors.New(r.Unavailable)
	}
	if !r.asset.valid(maxModpackArchive) || r.asset.Size == 0 {
		return PreparedModpack{}, errors.New("整合包版本没有可校验的下载文件")
	}
	filename, err := g.modpackAsset(ctx, r.asset, modpackHTTP)
	if err != nil {
		return PreparedModpack{}, err
	}
	return prepareModpack(ctx, filename, r.source)
}

func PrepareLocalModpack(ctx context.Context, filename string) (PreparedModpack, error) {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".mrpack":
		return prepareModpack(ctx, filename, ModrinthModpacks)
	case ".zip":
		return prepareModpack(ctx, filename, CurseForgeModpacks)
	default:
		return PreparedModpack{}, errors.New("请选择 Modrinth .mrpack 或 CurseForge 导出的 .zip 文件")
	}
}

func openModpack(filename string) (*os.File, int64, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, 0, err
	}
	info, err := f.Stat()
	if err == nil && (!info.Mode().IsRegular() || info.Size() == 0 || info.Size() > maxModpackArchive) {
		err = errors.New("整合包须为不超过 1 GiB 的普通文件")
	}
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	return f, info.Size(), nil
}

func modpackFingerprint(ctx context.Context, f *os.File, size int64) ([sha256.Size]byte, error) {
	var sum [sha256.Size]byte
	h := sha256.New()
	n, err := io.Copy(h, packReader{ctx, io.NewSectionReader(f, 0, size+1)})
	if err == nil && n != size {
		err = errors.New("读取期间整合包大小已变更")
	}
	copy(sum[:], h.Sum(nil))
	return sum, err
}

func prepareModpack(ctx context.Context, filename string, source ModpackSource) (PreparedModpack, error) {
	f, size, err := openModpack(filename)
	if err != nil {
		return PreparedModpack{}, err
	}
	defer f.Close()
	sum, err := modpackFingerprint(ctx, f, size)
	if err != nil {
		return PreparedModpack{}, err
	}
	plan, err := readModpack(f, size, source)
	if err != nil {
		return PreparedModpack{}, err
	}
	return PreparedModpack{info: plan.info, filename: filename, sum: sum}, ctx.Err()
}

func safeModpackPath(name string) bool {
	if name == "." || len(name) > 1024 || !fs.ValidPath(name) || !filepath.IsLocal(filepath.FromSlash(name)) ||
		strings.ContainsAny(name, "\\:\"<>|?*") || strings.ContainsFunc(name, unicode.IsControl) {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if len(part) > 255 || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return false
		}
		stem, _, _ := strings.Cut(part, ".")
		stem = strings.ToUpper(strings.TrimRight(stem, " "))
		switch stem {
		case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
			return false
		}
		for _, prefix := range []string{"COM", "LPT"} {
			if suffix, ok := strings.CutPrefix(stem, prefix); ok && slices.Contains([]string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "¹", "²", "³"}, suffix) {
				return false
			}
		}
	}
	return true
}

func readModpackManifest(f *zip.File, dst any) error {
	if f == nil {
		return errors.New("压缩包根目录缺少整合包清单")
	}
	if f.UncompressedSize64 > maxMetadata {
		return errors.New("整合包清单超出大小限制")
	}
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	data, err := io.ReadAll(io.LimitReader(r, maxMetadata+1))
	if err != nil {
		return err
	}
	if len(data) > maxMetadata {
		return errors.New("整合包清单超出大小限制")
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("无法读取整合包清单：%w", err)
	}
	return nil
}

func readModpack(r io.ReaderAt, size int64, source ModpackSource) (modpackPlan, error) {
	p := modpackPlan{info: ModpackInfo{Source: source}}
	z, err := zip.NewReader(r, size)
	if err != nil {
		return p, fmt.Errorf("无法打开整合包：%w", err)
	}
	if len(z.File) > maxModpackEntries {
		return p, errors.New("整合包包含过多文件")
	}
	files := make(map[string]*zip.File, len(z.File))
	seen := make(map[string]bool, len(z.File))
	var bytes uint64
	for _, f := range z.File {
		dir := f.Mode().IsDir() && strings.HasSuffix(f.Name, "/")
		name := strings.TrimSuffix(f.Name, "/")
		key := strings.ToLower(name)
		if !safeModpackPath(name) || seen[key] || (!dir && (!f.Mode().IsRegular() || f.Name != name)) {
			return p, fmt.Errorf("整合包包含不安全或重复的路径：%q", f.Name)
		}
		seen[key] = true
		if f.UncompressedSize64 > maxModpackFile || bytes+f.UncompressedSize64 > maxModpackBytes {
			return p, errors.New("整合包解压后超出大小限制")
		}
		bytes += f.UncompressedSize64
		if !dir {
			files[name] = f
		}
	}
	var layers []string
	var loader, loaderVersion string
	switch source {
	case ModrinthModpacks:
		var m mrpackManifest
		if err := readModpackManifest(files["modrinth.index.json"], &m); err != nil {
			return p, err
		}
		if m.FormatVersion != 1 || m.Game != "minecraft" {
			return p, errors.New("不支持此 Modrinth 清单格式")
		}
		p.info.Name, p.info.Version, p.info.Minecraft = m.Name, m.VersionID, m.Dependencies["minecraft"]
		for id, v := range m.Dependencies {
			switch id {
			case "minecraft":
			case "fabric-loader", "forge", "neoforge", "quilt-loader":
				if loader != "" {
					return p, errors.New("整合包清单同时指定了多个加载器")
				}
				loader, loaderVersion = id, v
			default:
				return p, fmt.Errorf("整合包要求不支持的加载器 %s", id)
			}
		}
		if len(m.Files) > maxModpackEntries {
			return p, errors.New("整合包依赖数量超出限制")
		}
		for _, f := range m.Files {
			switch f.Env.Client {
			case "optional", "unsupported":
				continue
			case "", "required":
			default:
				return p, fmt.Errorf("无法识别 %q 的客户端安装要求", f.Path)
			}
			if f.FileSize == nil {
				return p, fmt.Errorf("依赖 %q 缺少文件大小", f.Path)
			}
			a := modpackAsset{f.Downloads, *f.FileSize, "sha512", f.Hashes.SHA512}
			if !a.valid(maxModpackFile) {
				return p, fmt.Errorf("依赖 %q 缺少允许的下载地址或校验值", f.Path)
			}
			p.downloads = append(p.downloads, modpackDownload{f.Path, a})
		}
		layers = []string{"overrides", "client-overrides"}
	case CurseForgeModpacks:
		var m curseManifest
		if err := readModpackManifest(files["manifest.json"], &m); err != nil {
			return p, err
		}
		if m.ManifestType != "minecraftModpack" || m.ManifestVersion != 1 {
			return p, errors.New("ZIP 不是受支持的 CurseForge 整合包")
		}
		p.info.Name, p.info.Version, p.info.Minecraft = m.Name, m.Version, m.Minecraft.Version
		// A manifest can list fallback loaders. Prefer its one designated primary.
		loaders := m.Minecraft.ModLoaders
		n := slices.IndexFunc(loaders, func(l struct {
			ID      string
			Primary bool
		}) bool {
			return l.Primary
		})
		if n >= 0 {
			for _, l := range loaders[n+1:] {
				if l.Primary {
					return p, errors.New("CurseForge 清单指定了多个主加载器")
				}
			}
		} else if len(loaders) == 1 {
			n = 0
		} else if len(loaders) > 1 {
			return p, errors.New("CurseForge 清单没有指定主加载器")
		}
		if n >= 0 {
			kind, version, ok := strings.Cut(loaders[n].ID, "-")
			if !ok || kind == "" || version == "" {
				return p, fmt.Errorf("CurseForge 清单的加载器无效：%s", loaders[n].ID)
			}
			loader, loaderVersion = kind, version
		}
		if len(m.Files) > maxModpackEntries {
			return p, errors.New("整合包依赖数量超出限制")
		}
		for _, ref := range m.Files {
			if ref.Required == nil || *ref.Required {
				if ref.ProjectID <= 0 || ref.FileID <= 0 {
					return p, errors.New("CurseForge 清单包含无效的项目或文件编号")
				}
				p.curse = append(p.curse, ref)
			}
		}
		if m.Overrides == "" {
			m.Overrides = "overrides"
		}
		if !safeModpackPath(m.Overrides) {
			return p, errors.New("CurseForge 清单的覆盖目录无效")
		}
		layers = []string{m.Overrides}
	default:
		return p, errors.New("未知整合包来源")
	}
	p.info.Loader, p.info.LoaderVersion, err = normalizeLoader(p.info.Minecraft, loader, loaderVersion)
	if err != nil {
		return p, err
	}
	for _, layer := range layers {
		for _, f := range z.File {
			name, ok := strings.CutPrefix(f.Name, layer+"/")
			if ok && !f.Mode().IsDir() {
				p.overrides = append(p.overrides, modpackOverride{name, f})
			}
		}
	}
	p.info.Downloads, p.info.Overrides = len(p.downloads)+len(p.curse), len(p.overrides)
	return p, p.validate()
}

func (p modpackPlan) validate() error {
	type entry struct {
		path string
		file bool
	}
	paths := make(map[string]entry)
	var total int64
	add := func(name string, overwrite bool, size int64) error {
		total += size
		if total > maxModpackBytes || !safeModpackPath(name) {
			return fmt.Errorf("整合包超出大小限制或包含无效路径：%q", name)
		}
		key := strings.ToLower(name)
		if old, ok := paths[key]; ok && (!overwrite || old != (entry{name, true})) {
			return fmt.Errorf("整合包包含冲突路径：%q", name)
		}
		paths[key] = entry{name, true}
		for dir := path.Dir(name); dir != "."; dir = path.Dir(dir) {
			key := strings.ToLower(dir)
			if old, ok := paths[key]; ok && old != (entry{dir, false}) {
				return fmt.Errorf("整合包包含冲突目录：%q", dir)
			}
			paths[key] = entry{dir, false}
		}
		return nil
	}
	for _, d := range p.downloads {
		if err := add(d.path, false, d.asset.Size); err != nil {
			return err
		}
	}
	for _, o := range p.overrides {
		if err := add(o.path, true, int64(o.file.UncompressedSize64)); err != nil {
			return err
		}
	}
	return nil
}

// InstallModpack installs and validates all files before publishing the instance.
// The caller serializes state writes and keeps the submitted state snapshot.
func (g Game) InstallModpack(ctx context.Context, store Store, next State, i Instance, pack PreparedModpack, key string, report func(Progress)) error {
	install := func(ctx context.Context, work Instance, report func(Progress)) error {
		return g.InstallWithJava(ctx, work, next.Settings.JavaPath, report)
	}
	return g.installModpack(ctx, store, next, i, pack, newCurseForge(key), modpackHTTP, install, report)
}

func (g Game) installModpack(ctx context.Context, store Store, next State, i Instance, pack PreparedModpack, cf curseForge,
	client *http.Client, install func(context.Context, Instance, func(Progress)) error, report func(Progress)) error {
	saved, exists := next.Find(i.ID)
	if !exists || saved != i || i.Version != pack.info.Minecraft || !strings.EqualFold(i.Loader, pack.info.Loader) ||
		i.LoaderVersion != pack.info.LoaderVersion || !filepath.IsAbs(i.GameDir) {
		return errors.New("整合包的实例配置已变更")
	}
	if err := supported(i); err != nil {
		return err
	}
	if report == nil {
		report = func(Progress) {}
	}
	report(Progress{Stage: "检查整合包清单"})
	f, size, err := openModpack(pack.filename)
	if err != nil {
		return err
	}
	defer f.Close()
	sum, err := modpackFingerprint(ctx, f, size)
	if err != nil {
		return err
	}
	if sum != pack.sum {
		return errors.New("整合包文件已变更，请重新选择")
	}
	plan, err := readModpack(f, size, pack.info.Source)
	if err != nil {
		return err
	}
	if len(plan.curse) != 0 {
		report(Progress{Stage: "解析 CurseForge 依赖"})
		plan.downloads, err = cf.resolve(ctx, plan.curse)
		if err != nil {
			return err
		}
		if err := plan.validate(); err != nil {
			return err
		}
	}
	parent, dest := filepath.Dir(i.GameDir), filepath.Base(i.GameDir)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return err
	}
	base, err := os.OpenRoot(parent)
	if err != nil {
		return err
	}
	defer base.Close()
	checkDest := func() error {
		if _, err := base.Lstat(dest); !errors.Is(err, os.ErrNotExist) {
			if err == nil {
				err = errors.New("目标实例目录已存在，请保留旧目录并更换实例存放位置")
			}
			return err
		}
		return nil
	}
	if err := checkDest(); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(parent, ".lxmcl-modpack-")
	if err != nil {
		return err
	}
	stage := filepath.Base(tmp)
	defer base.RemoveAll(stage)
	root, err := base.OpenRoot(stage)
	if err != nil {
		return err
	}
	err = g.fillModpack(ctx, root, plan, client, report)
	if err := errors.Join(err, root.Close()); err != nil {
		return err
	}
	work := i
	work.GameDir = tmp
	if err := install(ctx, work, report); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := checkDest(); err != nil {
		return err
	}
	report(Progress{Stage: "保存新实例"})
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := base.Rename(stage, dest); err != nil {
		return err
	}
	if err := store.Save(next); err != nil {
		return errors.Join(err, base.Rename(dest, stage))
	}
	return nil
}

func (g Game) fillModpack(ctx context.Context, root *os.Root, p modpackPlan, client *http.Client, report func(Progress)) error {
	write := func(name string, overwrite bool, copy func(io.Writer) error) error {
		if err := root.MkdirAll(filepath.FromSlash(path.Dir(name)), 0755); err != nil {
			return err
		}
		flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
		if overwrite {
			flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
		}
		out, err := root.OpenFile(filepath.FromSlash(name), flags, 0644)
		if err != nil {
			return err
		}
		return errors.Join(copy(out), out.Close())
	}
	stage := "下载并校验整合包依赖"
	report(Progress{Stage: stage, Total: len(p.downloads)})
	if err := batch(ctx, p.downloads, func(ctx context.Context, d modpackDownload) error {
		file, err := g.modpackAsset(ctx, d.asset, client)
		if err == nil {
			err = write(d.path, false, func(out io.Writer) error {
				f, err := os.Open(file)
				if err != nil {
					return err
				}
				defer f.Close()
				return d.asset.copyVerified(ctx, out, f)
			})
		}
		if err != nil {
			return fmt.Errorf("%s：%w", d.path, err)
		}
		return nil
	}, func(done int) { report(Progress{Stage: stage, Done: done, Total: len(p.downloads)}) }); err != nil {
		return err
	}
	stage = "解压整合包配置"
	report(Progress{Stage: stage, Total: len(p.overrides)})
	for n, o := range p.overrides {
		err := write(o.path, true, func(out io.Writer) error {
			r, err := o.file.Open()
			if err != nil {
				return err
			}
			defer r.Close()
			size := int64(o.file.UncompressedSize64)
			count, err := io.Copy(out, io.LimitReader(packReader{ctx, r}, size+1))
			if err == nil && count != size {
				err = errors.New("压缩条目的大小与清单不匹配")
			}
			return err
		})
		if err != nil {
			return fmt.Errorf("%s：%w", o.path, err)
		}
		report(Progress{Stage: stage, Done: n + 1, Total: len(p.overrides)})
	}
	return ctx.Err()
}

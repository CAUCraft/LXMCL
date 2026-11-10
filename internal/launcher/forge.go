package launcher

import (
	"archive/zip"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"time"
)

const (
	forgeMaven = "https://maven.minecraftforge.net/"
	neoMaven   = "https://maven.neoforged.net/releases/"
)

var javaClass = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$.]{0,255}$`)

type forgeLibrary struct {
	Name      string
	Rules     []rule
	Downloads struct{ Artifact artifact }
}

type forgeProfile struct {
	Platform, ID, InheritsFrom, MainClass, LoaderVersion string
	Arguments                                            fabricArguments
	Libraries                                            []forgeLibrary
	Auxiliary                                            []forgeLibrary `json:",omitempty"`
}

type forgeInstaller struct {
	Minecraft, Version, JSON string
	Libraries                []forgeLibrary
	Data                     map[string]struct{ Client string }
	Processors               []struct {
		Sides   []string
		Outputs map[string]string
	}
}

func forgeInstallerURL(i Instance) (string, error) {
	loader, version, err := normalizeLoader(i.Version, i.Loader, i.LoaderVersion)
	if err != nil {
		return "", err
	}
	coordinate := i.Version + "-" + version
	switch loader {
	case LoaderForge:
		return forgeMaven + "net/minecraftforge/forge/" + coordinate + "/forge-" + coordinate + "-installer.jar", nil
	case LoaderNeoForge:
		if i.Version == "1.20.1" {
			return neoMaven + "net/neoforged/forge/" + coordinate + "/forge-" + coordinate + "-installer.jar", nil
		}
		return neoMaven + "net/neoforged/neoforge/" + version + "/neoforge-" + version + "-installer.jar", nil
	default:
		return "", errors.New("需要 Forge 或 NeoForge 加载器")
	}
}

func (g Game) forgeDir(i Instance) string {
	_, version, _ := normalizeLoader(i.Version, i.Loader, i.LoaderVersion)
	return filepath.Join(g.Cache, strings.ToLower(i.Loader), i.Version, version, strings.ReplaceAll(platformKey(), "/", "-"))
}

func forgeMetadata(path string) (forgeInstaller, forgeProfile, error) {
	var installer forgeInstaller
	var profile forgeProfile
	z, err := zip.OpenReader(path)
	if err != nil {
		return installer, profile, err
	}
	defer z.Close()
	entry := func(name string) *zip.File {
		for _, f := range z.File {
			if f.Name == name && f.Mode().IsRegular() {
				return f
			}
		}
		return nil
	}
	if err := readModpackManifest(entry("install_profile.json"), &installer); err != nil {
		return installer, profile, fmt.Errorf("无法读取官方安装器清单：%w", err)
	}
	name := strings.TrimPrefix(installer.JSON, "/")
	if !safeModpackPath(name) || filepath.Ext(name) != ".json" {
		return installer, profile, errors.New("官方安装器缺少受支持的客户端配置")
	}
	if err := readModpackManifest(entry(name), &profile); err != nil {
		return installer, profile, fmt.Errorf("无法读取官方客户端配置：%w", err)
	}
	return installer, profile, nil
}

func validateForgeProfile(i Instance, p forgeProfile) error {
	if p.InheritsFrom != i.Version || !safeModpackPath(p.ID) || strings.Contains(p.ID, "/") ||
		!javaClass.MatchString(p.MainClass) || len(p.Libraries) == 0 {
		return fmt.Errorf("%s 客户端配置不匹配 Minecraft %s", i.LoaderName(), i.Version)
	}
	return nil
}

func (d *downloads) fetchSHA1(ctx context.Context, source string) (string, error) {
	var sha string
	err := d.fetch(ctx, source+".sha1", func(ctx context.Context, resp *http.Response) error {
		data, err := io.ReadAll(io.LimitReader(cancelReader{ctx, resp.Body}, 128))
		if err != nil {
			return err
		}
		s := strings.ToLower(strings.TrimSpace(string(data)))
		if _, err := (artifact{SHA: s, Size: unknownSize}).checksum(); err != nil {
			return err
		}
		sha = s
		return nil
	})
	return sha, err
}

// The installer's download runs from an empty Mojang-style directory. It never
// receives the modpack directory, launcher credentials, or the user's profiles.
func (g Game) installForge(ctx context.Context, i Instance, m metadata, javaPath string, d *downloads, report func(Progress)) error {
	dest := g.forgeDir(i)
	if p, err := g.readForge(i); err == nil {
		if _, err := g.verifyForge(ctx, i, p); err == nil {
			return nil
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	report(Progress{Stage: "检查加载器所需的 Java"})
	j, err := FindJava(ctx, i.Launch.JavaExecutable(javaPath), m.JavaVersion.MajorVersion)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%w：%s 安装需要 Java %d：%w", ErrJavaRequired, i.LoaderName(), m.JavaVersion.MajorVersion, err)
	}
	j.Path = javaConsole(j.Path)
	src, err := forgeInstallerURL(i)
	if err != nil {
		return err
	}
	report(Progress{Stage: "下载并校验 " + i.Loader + " 安装器"})
	sha, err := d.fetchSHA1(ctx, src)
	if err != nil {
		return fmt.Errorf("无法获取 %s 安装器校验值：%w", i.LoaderName(), err)
	}
	// A modern installer can embed large binary patches.
	installerJar := filepath.Join(g.Cache, "installers", sha+".jar")
	if err := d.fileBound(ctx, installerJar, artifact{URL: src, SHA: sha, Size: unknownSize}, 128<<20); err != nil {
		return fmt.Errorf("%s 安装器：%w", i.LoaderName(), err)
	}
	installer, p, err := forgeMetadata(installerJar)
	if err != nil {
		return err
	}
	if err := validateForgeProfile(i, p); err != nil {
		return err
	}
	if installer.Minecraft != i.Version || installer.Version != p.ID {
		return fmt.Errorf("%s 安装器与所选 Minecraft 版本不匹配", i.LoaderName())
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(dest), ".installer-*")
	if err != nil {
		return err
	}
	keepStage := false
	defer func() {
		if !keepStage {
			os.RemoveAll(stage)
		}
	}()
	game := filepath.Join(stage, "install")
	base := filepath.Join(game, "versions", i.Version)
	if err := os.MkdirAll(base, 0755); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(game, "launcher_profiles.json"), []byte(`{"profiles":{}}`)); err != nil {
		return err
	}
	for name, target := range map[string]string{"client.jar": i.Version + ".jar", "version.json": i.Version + ".json"} {
		if err := copyForgeFile(ctx, filepath.Join(base, target), filepath.Join(g.versionDir(i.Version), name)); err != nil {
			return err
		}
	}
	if err := g.prefetchForgeLibraries(ctx, d, game, installer.Libraries, p.Libraries, report); err != nil {
		return err
	}
	report(Progress{Stage: "运行 " + i.Loader + " 官方客户端安装器"})
	if err := g.runForgeInstaller(ctx, j.Path, installerJar, game, i); err != nil {
		return err
	}
	var installed forgeProfile
	ok, err := readJSON(filepath.Join(game, "versions", p.ID, p.ID+".json"), &installed)
	if err != nil || !ok || installed.ID != p.ID || validateForgeProfile(i, installed) != nil {
		return fmt.Errorf("%s 安装器未生成有效的客户端配置", i.LoaderName())
	}
	// The version JSON itself is taken from the verified installer. Preserve
	// only the launch libraries from the staging area, including generated jars.
	report(Progress{Stage: "校验 " + i.Loader + " 启动文件"})
	p.Platform, p.LoaderVersion = platformKey(), i.LoaderVersion
	libraryDir := filepath.Join(game, "libraries")
	p.Libraries, err = attestForgeLibraries(ctx, libraryDir, p.Libraries)
	if err != nil {
		return err
	}
	extra, err := installer.clientLibraries()
	if err != nil {
		return err
	}
	p.Auxiliary, err = attestForgeLibraries(ctx, libraryDir, extra)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if err := writeFile(filepath.Join(game, "verified.json"), encoded); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Move only our own cache entry out of the way while publishing its replacement.
	old := filepath.Join(stage, "old")
	if err := renameForgeCache(ctx, dest, old); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := renameForgeCache(ctx, game, dest); err != nil {
		if restore := renameForgeCache(context.WithoutCancel(ctx), old, dest); restore != nil && !errors.Is(restore, os.ErrNotExist) {
			keepStage = true
			return errors.Join(err, restore)
		}
		return err
	}
	return nil
}

func renameForgeCache(ctx context.Context, from, to string) error {
	var err error
	for attempt := range 6 {
		if err := ctx.Err(); err != nil {
			return err
		}
		err = os.Rename(from, to)
		if err == nil || runtime.GOOS != "windows" || attempt == 5 ||
			(!errors.Is(err, os.ErrPermission) && !errors.Is(err, syscall.Errno(32))) {
			return err
		}
		// Windows scanners may briefly hold newly installed JARs open.
		if err := waitContext(ctx, time.Duration(1<<attempt)*50*time.Millisecond); err != nil {
			return err
		}
	}
	return err
}

func (g Game) prefetchForgeLibraries(ctx context.Context, d *downloads, game string, installer, client []forgeLibrary, report func(Progress)) error {
	seen := make(map[string]artifact)
	var files []fileDownload
	for _, lib := range append(slices.Clone(installer), client...) {
		a := lib.Downloads.Artifact
		if a.URL == "" {
			continue // embedded in the installer or produced by a client processor
		}
		u, err := url.Parse(a.URL)
		_, neoAsset := neoMavenPath(a.URL)
		if err != nil || (!officialURL(u) && !neoAsset) || !safeModpackPath(a.Path) || a.Size < 0 || a.Size > maxModpackFile {
			return fmt.Errorf("无效的加载器运行库：%s", lib.Name)
		}
		if a.Size == 0 {
			a.Size = unknownSize
		}
		if prev, exists := seen[a.Path]; exists {
			if !strings.EqualFold(prev.SHA, a.SHA) || prev.Size != a.Size {
				return fmt.Errorf("加载器运行库校验值冲突：%s", a.Path)
			}
			continue
		}
		seen[a.Path] = a
		files = append(files, fileDownload{filepath.Join(game, "libraries", filepath.FromSlash(a.Path)), a})
	}
	const stage = "下载并校验加载器运行库"
	report(Progress{Stage: d.mirrorStage(stage), Total: len(files)})
	return batch(ctx, files, func(ctx context.Context, f fileDownload) error {
		sum, err := f.file.checksum()
		if err != nil {
			return fmt.Errorf("%s：%w", f.file.Path, err)
		}
		cache := filepath.Join(g.Cache, "loader-libraries", hex.EncodeToString(sum))
		base := filepath.Join(g.Cache, "libraries", filepath.FromSlash(f.file.Path))
		if ok, err := checkFileBound(ctx, base, f.file, sum, 128<<20); ok && err == nil {
			cache = base
		} else {
			if err := d.fileBound(ctx, cache, f.file, 128<<20); err != nil {
				return fmt.Errorf("%s：%w", f.file.Path, err)
			}
		}
		if err := os.MkdirAll(filepath.Dir(f.path), 0755); err != nil {
			return err
		}
		return copyForgeFile(ctx, f.path, cache)
	}, func(done int) { report(Progress{Stage: d.mirrorStage(stage), Done: done, Total: len(files)}) })
}

func copyForgeFile(ctx context.Context, dest, source string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, cancelReader{ctx, in})
	return errors.Join(err, out.Close())
}

func (g Game) runForgeInstaller(ctx context.Context, java, installer, game string, i Instance) error {
	dir := filepath.Join(g.Cache, "installer-logs")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	log, err := os.CreateTemp(dir, strings.ToLower(i.Loader)+"-*.log")
	if err != nil {
		return err
	}
	defer log.Close()
	ctx, stop := context.WithTimeout(ctx, 15*time.Minute)
	defer stop()
	cmd := exec.CommandContext(ctx, java, "-Djava.awt.headless=true", "-Duser.home="+game,
		"-jar", installer, "--installClient", game)
	hideConsole(cmd)
	cmd.Dir, cmd.Stdout, cmd.Stderr = game, log, log
	// Do not pass key/token environment variables or inherited Java agents.
	cmd.Env = javaEnvironment(true)
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return context.Canceled
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("%s 安装器运行超时（日志：%s）：%w", i.LoaderName(), log.Name(), ctx.Err())
		}
		return fmt.Errorf("%s 安装器运行失败：%w（日志：%s）", i.LoaderName(), err, log.Name())
	}
	return nil
}

func forgeLibraryPath(lib forgeLibrary) (string, error) {
	if lib.Downloads.Artifact.Path != "" {
		if p := lib.Downloads.Artifact.Path; safeModpackPath(p) && filepath.Ext(p) == ".jar" {
			return p, nil
		}
		return "", fmt.Errorf("无效的加载器运行库路径：%s", lib.Downloads.Artifact.Path)
	}
	return mavenLibraryPath(lib.Name)
}

func attestForgeLibraries(ctx context.Context, root string, libs []forgeLibrary) ([]forgeLibrary, error) {
	result := make([]forgeLibrary, 0, len(libs))
	seen := make(map[string]bool)
	for _, lib := range libs {
		if !allowed(lib.Rules, nil) {
			continue
		}
		path, err := forgeLibraryPath(lib)
		if err != nil {
			return nil, err
		}
		if seen[path] {
			continue
		}
		seen[path] = true
		full := filepath.Join(root, filepath.FromSlash(path))
		st, err := os.Lstat(full)
		if err != nil || !st.Mode().IsRegular() || st.Size() <= 0 || st.Size() > maxModpackFile {
			return nil, fmt.Errorf("安装器未生成有效的运行库：%s", path)
		}
		a := lib.Downloads.Artifact
		if a.SHA != "" {
			if a.Size == 0 {
				a.Size = st.Size()
			}
			if err := verifyArtifact(ctx, fileDownload{full, a}); err != nil {
				return nil, err
			}
		} else {
			// Some processor outputs don't have a hash in version.json. The
			// official installer checks its own outputs before reporting success.
			f, err := os.Open(full)
			if err != nil {
				return nil, err
			}
			h := sha1.New()
			n, err := io.Copy(h, io.LimitReader(cancelReader{ctx, f}, st.Size()+1))
			f.Close()
			if err != nil || n != st.Size() {
				return nil, fmt.Errorf("无法校验安装器产物 %s：%w", path, errors.Join(err, io.ErrUnexpectedEOF))
			}
			a.SHA, a.Size = hex.EncodeToString(h.Sum(nil)), st.Size()
		}
		a.Path = path
		lib.Downloads.Artifact = a
		result = append(result, lib)
	}
	return result, nil
}

// Forge discovers the patched client and some libraries through libraryDirectory
// rather than through the version profile's classpath.
func (p forgeInstaller) clientLibraries() ([]forgeLibrary, error) {
	value := func(raw string) string {
		if strings.HasPrefix(raw, "{") && strings.HasSuffix(raw, "}") {
			return p.Data[raw[1:len(raw)-1]].Client
		}
		return raw
	}
	generated := make(map[string]string)
	add := func(raw, sha string) error {
		coord := strings.TrimSuffix(strings.TrimPrefix(raw, "["), "]")
		if "["+coord+"]" != raw || (strings.Contains(coord, "@") && !strings.HasSuffix(coord, "@jar")) {
			return nil
		}
		path, err := mavenLibraryPath(coord)
		if err != nil {
			return err
		}
		if sha != "" {
			sha = strings.Trim(sha, "'")
			if _, err := (artifact{SHA: sha, Size: unknownSize}).checksum(); err != nil {
				return fmt.Errorf("安装器产物 %s 缺少有效校验值：%w", path, err)
			}
		}
		if old := generated[path]; old != "" {
			if sha == "" || strings.EqualFold(old, sha) {
				return nil
			}
			return fmt.Errorf("安装器产物 %s 的校验值冲突", path)
		}
		generated[path] = sha
		return nil
	}
	for _, step := range p.Processors {
		if len(step.Sides) != 0 && !slices.Contains(step.Sides, "client") {
			continue
		}
		for dest, sha := range step.Outputs {
			if err := add(value(dest), value(sha)); err != nil {
				return nil, err
			}
		}
	}
	for _, key := range []string{"PATCHED", "MC_EXTRA"} {
		if err := add(p.Data[key].Client, ""); err != nil {
			return nil, err
		}
	}
	out := make([]forgeLibrary, 0, len(generated)+len(p.Libraries))
	for path, sha := range generated {
		var lib forgeLibrary
		lib.Downloads.Artifact = artifact{Path: path, SHA: sha}
		out = append(out, lib)
	}
	for _, lib := range p.Libraries {
		a := lib.Downloads.Artifact
		if a.URL != "" && strings.HasSuffix(a.Path, ".jar") {
			out = append(out, lib)
		}
	}
	return out, nil
}

func (g Game) readForge(i Instance) (forgeProfile, error) {
	var p forgeProfile
	ok, err := readJSON(filepath.Join(g.forgeDir(i), "verified.json"), &p)
	if err != nil {
		return p, err
	}
	if !ok || p.Platform != platformKey() || p.LoaderVersion != i.LoaderVersion {
		return p, fmt.Errorf("%s 文件尚未校验", i.LoaderName())
	}
	return p, validateForgeProfile(i, p)
}

func (g Game) verifyForge(ctx context.Context, i Instance, p forgeProfile) ([]string, error) {
	root := filepath.Join(g.forgeDir(i), "libraries")
	var files []fileDownload
	var cp []string
	for n, lib := range append(slices.Clone(p.Libraries), p.Auxiliary...) {
		path, err := forgeLibraryPath(lib)
		if err != nil {
			return nil, err
		}
		full := filepath.Join(root, filepath.FromSlash(path))
		files = append(files, fileDownload{full, lib.Downloads.Artifact})
		if n < len(p.Libraries) {
			cp = append(cp, full)
		}
	}
	return cp, batch(ctx, files, verifyArtifact, nil)
}

func mavenLibraryPath(name string) (string, error) {
	name, ext, hasExt := strings.Cut(name, "@")
	if hasExt && ext != "jar" {
		return "", fmt.Errorf("无效的 Java 运行库类型：%s", ext)
	}
	parts := strings.Split(name, ":")
	if len(parts) != 3 && len(parts) != 4 {
		return "", fmt.Errorf("无效的 Java 运行库坐标：%s", name)
	}
	groups := strings.Split(parts[0], ".")
	for _, part := range append(groups, parts[1:]...) {
		if !mavenPart.MatchString(part) {
			return "", fmt.Errorf("无效的 Java 运行库坐标：%s", name)
		}
	}
	return strings.Join(groups, "/") + "/" + parts[1] + "/" + parts[2] + "/" + strings.Join(parts[1:], "-") + ".jar", nil
}

func mavenLibraryKey(name string) string {
	name, _, _ = strings.Cut(name, "@")
	parts := strings.Split(name, ":")
	switch len(parts) {
	case 3:
		return parts[0] + ":" + parts[1]
	case 4:
		return parts[0] + ":" + parts[1] + ":" + parts[3]
	}
	return ""
}

func (g Game) mergeForgeClasspath(m metadata, p forgeProfile, base, loader []string) []string {
	replaced := make(map[string]bool)
	for _, lib := range p.Libraries {
		if k := mavenLibraryKey(lib.Name); k != "" {
			replaced[k] = true
		}
	}
	skip := make(map[string]bool)
	for _, lib := range m.Libraries {
		if replaced[mavenLibraryKey(lib.Name)] {
			skip[filepath.Join(g.Cache, "libraries", filepath.FromSlash(lib.Downloads.Artifact.Path))] = true
		}
	}
	return append(loader, slices.DeleteFunc(base, func(path string) bool { return skip[path] })...)
}

func ignoreCachedVanillaJar(jvm []string, version string) {
	const flag = "-DignoreList="
	for n, arg := range jvm {
		if list, ok := strings.CutPrefix(arg, flag); ok {
			// Forge's inherited profile names the vanilla jar like Mojang's
			// launcher does. LXMCL stores that same verified artifact as client.jar.
			names := strings.Split(list, ",")
			if slices.Contains(names, version+".jar") && !slices.Contains(names, "client.jar") {
				jvm[n] = arg + ",client.jar"
			}
		}
	}
}

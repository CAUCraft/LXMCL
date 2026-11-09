package launcher

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

const manifestURL = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"

var (
	versionID = regexp.MustCompile(`^[0-9][0-9A-Za-z._-]{0,63}$`)
	releaseID = regexp.MustCompile(`^[0-9]{1,3}\.[0-9]{1,3}(\.[0-9]{1,3})?$`)
	playerID  = regexp.MustCompile(`^[A-Za-z0-9_]{3,16}$`)
	variable  = regexp.MustCompile(`\$\{([^}]+)\}`)
)

type Game struct {
	Cache string
}

type Version struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	URL  string `json:"url"`
	SHA  string `json:"sha1"`
}

type artifact struct {
	Path string `json:"path"`
	URL  string `json:"url"`
	SHA  string `json:"sha1"`
	Size int64  `json:"size"`
}

type rule struct {
	Action   string
	Features map[string]bool
	OS       struct{ Name, Arch, Version string }
}

type metadata struct {
	ID, Type, MainClass string
	Arguments           struct{ JVM, Game []json.RawMessage }
	JavaVersion         struct{ MajorVersion int }
	Downloads           struct{ Client artifact }
	AssetIndex          struct {
		ID string
		artifact
	}
	Libraries []struct {
		Rules     []rule
		Downloads struct{ Artifact artifact }
		Natives   map[string]string
	}
	Logging struct {
		Client struct {
			Argument string
			File     struct {
				ID string
				artifact
			}
		}
	}
}

type assetIndex struct {
	Objects map[string]struct {
		Hash string
		Size int64
	}
}

type Progress struct {
	Stage       string
	Done, Total int
}

func Catalog(ctx context.Context) ([]Version, error) {
	return (&downloads{http: officialHTTP}).catalog(ctx)
}

func (d *downloads) catalog(ctx context.Context) ([]Version, error) {
	var data struct{ Versions []Version }
	if err := retry(ctx, func() error {
		resp, err := d.get(ctx, manifestURL)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		return json.NewDecoder(io.LimitReader(resp.Body, maxMetadata)).Decode(&data)
	}); err != nil {
		return nil, err
	}
	if len(data.Versions) == 0 {
		return nil, errors.New("官方版本目录为空")
	}
	return data.Versions, nil
}

func modernRelease(id string) bool {
	if !releaseID.MatchString(id) {
		return false
	}
	var major, minor, patch int
	if n, _ := fmt.Sscanf(id, "%d.%d.%d", &major, &minor, &patch); n < 2 {
		return false
	}
	return major > 1 || (major == 1 && (minor > 20 || (minor == 20 && patch >= 5)))
}

func Releases(ctx context.Context) ([]Version, error) {
	return (&downloads{http: officialHTTP}).releases(ctx)
}

func (d *downloads) releases(ctx context.Context) ([]Version, error) {
	all, err := d.catalog(ctx)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(all, func(v Version) bool {
		return v.Type != "release" || !modernRelease(v.ID)
	}), nil
}

func allowed(rules []rule, features map[string]bool) bool {
	if len(rules) == 0 {
		return true
	}
	osName, arch := runtime.GOOS, runtime.GOARCH
	if osName == "darwin" {
		osName = "osx"
	}
	switch arch {
	case "amd64":
		arch = "x86_64"
	case "386":
		arch = "x86"
	case "arm64":
		arch = "aarch64"
	}
	ok := false
	for _, r := range rules {
		if r.OS.Name != "" && r.OS.Name != osName || r.OS.Version != "" {
			continue
		}
		if r.OS.Arch != "" {
			archMatches, _ := regexp.MatchString("^(?:"+r.OS.Arch+")$", arch)
			if !archMatches {
				continue
			}
		}
		matches := true
		for feature, want := range r.Features {
			if features[feature] != want {
				matches = false
				break
			}
		}
		if matches {
			ok = r.Action == "allow"
		}
	}
	return ok
}

func gameArgs(raw []json.RawMessage, values map[string]string, features map[string]bool) ([]string, error) {
	var args []string
	for _, item := range raw {
		var s string
		if json.Unmarshal(item, &s) == nil {
			args = append(args, s)
			continue
		}
		var entry struct {
			Rules []rule
			Value json.RawMessage
		}
		if err := json.Unmarshal(item, &entry); err != nil {
			return nil, err
		}
		if !allowed(entry.Rules, features) {
			continue
		}
		if json.Unmarshal(entry.Value, &s) == nil {
			args = append(args, s)
		} else {
			var list []string
			if err := json.Unmarshal(entry.Value, &list); err != nil {
				return nil, err
			}
			args = append(args, list...)
		}
	}
	for i, s := range args {
		var unknown string
		args[i] = variable.ReplaceAllStringFunc(s, func(token string) string {
			value, ok := values[token[2:len(token)-1]]
			if !ok {
				unknown = token
			}
			return value
		})
		if unknown != "" {
			return nil, fmt.Errorf("不支持的启动参数 %s", unknown)
		}
	}
	return args, nil
}

func (g Game) versionDir(id string) string { return filepath.Join(g.Cache, "versions", id) }

func supported(i Instance) error {
	if !strings.EqualFold(i.Loader, "vanilla") && i.Loader != "" {
		return errors.New("目前仅支持原版 Java 版；此实例使用了其他加载器")
	}
	if !versionID.MatchString(i.Version) || !modernRelease(i.Version) || !filepath.IsAbs(i.GameDir) {
		return errors.New("请选择 1.20.5 或更新的原版正式版本，并使用绝对游戏目录")
	}
	return nil
}

func (g Game) Installed(i Instance) bool {
	if supported(i) != nil {
		return false
	}
	if _, err := g.readReceipt(i.Version); err != nil {
		return false
	}
	for _, name := range []string{"version.json", "client.jar"} {
		st, err := os.Stat(filepath.Join(g.versionDir(i.Version), name))
		if err != nil || !st.Mode().IsRegular() || st.Size() == 0 {
			return false
		}
	}
	return true
}

var (
	// ErrRepairRequired indicates that cached files cannot be used to start the client.
	ErrRepairRequired = errors.New("游戏文件需要重新校验")
	// ErrJavaRequired indicates that the selected game needs a usable Java runtime.
	ErrJavaRequired = errors.New("需要兼容的 Java 运行时")
)

type installReceipt struct {
	Platform   string `json:"platform"`
	VersionSHA string `json:"version_sha1"`
}

func platformKey() string { return runtime.GOOS + "/" + runtime.GOARCH }

func (g Game) readReceipt(id string) (installReceipt, error) {
	var r installReceipt
	f, err := os.Open(filepath.Join(g.versionDir(id), "verified"))
	if err != nil {
		return r, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 1025))
	if err != nil {
		return r, err
	}
	if len(b) > 1024 {
		return r, errors.New("安装校验记录损坏")
	}
	// Existing installations did not persist the catalog SHA-1. Continue checking
	// their complete artifacts, and upgrade the receipt after an explicit repair.
	if strings.TrimSpace(string(b)) == "1" {
		return r, nil
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return r, errors.New("安装校验记录损坏")
	}
	if r.Platform != platformKey() {
		return r, errors.New("已安装的游戏文件属于其他平台")
	}
	if _, err := (artifact{SHA: r.VersionSHA, Size: unknownSize}).checksum(); err != nil {
		return r, errors.New("安装校验记录缺少版本摘要")
	}
	return r, nil
}

func (g Game) readVersion(id, expectedSHA string) (metadata, error) {
	var m metadata
	f, err := os.Open(filepath.Join(g.versionDir(id), "version.json"))
	if err != nil {
		return m, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxMetadata+1))
	if err != nil {
		return m, err
	}
	sum := sha1.Sum(b)
	if len(b) > maxMetadata || (expectedSHA != "" && !strings.EqualFold(hex.EncodeToString(sum[:]), expectedSHA)) {
		return m, errors.New("版本元数据未通过校验")
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return m, fmt.Errorf("无法读取版本元数据：%w", err)
	}
	if m.ID != id || m.Type != "release" || m.MainClass == "" || len(m.Arguments.JVM) == 0 || m.JavaVersion.MajorVersion < 21 {
		return m, errors.New("版本元数据缺失或不受支持")
	}
	return m, nil
}

func (g Game) binaries(m metadata) ([]fileDownload, []string, error) {
	client := filepath.Join(g.versionDir(m.ID), "client.jar")
	files := []fileDownload{{client, m.Downloads.Client}}
	seen := make(map[string]artifact)
	var cp []string
	for _, lib := range m.Libraries {
		if !allowed(lib.Rules, nil) {
			continue
		}
		if len(lib.Natives) != 0 {
			return nil, nil, errors.New("当前版本使用不受支持的旧版本机库格式")
		}
		file := lib.Downloads.Artifact
		if !filepath.IsLocal(file.Path) || filepath.Clean(file.Path) == "." {
			return nil, nil, fmt.Errorf("无效的库路径：%q", file.Path)
		}
		path := filepath.Join(g.Cache, "libraries", filepath.FromSlash(file.Path))
		key := path
		if runtime.GOOS == "windows" {
			key = strings.ToLower(path)
		}
		if prev, ok := seen[key]; ok {
			if !strings.EqualFold(prev.SHA, file.SHA) || prev.Size != file.Size {
				return nil, nil, fmt.Errorf("版本元数据中的库文件冲突：%s", file.Path)
			}
			continue
		}
		seen[key] = file
		files = append(files, fileDownload{path, file})
		cp = append(cp, path)
	}
	if m.Logging.Client.Argument != "" {
		f := m.Logging.Client.File
		if !filepath.IsLocal(f.ID) || filepath.Base(f.ID) != f.ID {
			return nil, nil, errors.New("无效的日志配置文件名")
		}
		files = append(files, fileDownload{filepath.Join(g.Cache, "assets", "log_configs", f.ID), f.artifact})
	}
	return files, append(cp, client), nil
}

func (g Game) indexPath(m metadata) (string, error) {
	if !versionID.MatchString(m.AssetIndex.ID) {
		return "", errors.New("无效的资源索引")
	}
	return filepath.Join(g.Cache, "assets", "indexes", m.AssetIndex.ID+".json"), nil
}

func (g Game) assets(indexPath string) ([]fileDownload, error) {
	var assets assetIndex
	if found, err := readJSON(indexPath, &assets); err != nil || !found {
		return nil, errors.Join(errors.New("无法读取资源索引"), err)
	}
	files := make([]fileDownload, 0, len(assets.Objects))
	seen := make(map[string]int64)
	for _, a := range assets.Objects {
		hash := strings.ToLower(a.Hash)
		obj := artifact{SHA: hash, Size: a.Size}
		if _, err := obj.checksum(); err != nil || a.Size < 0 {
			return nil, errors.New("资源索引包含无效摘要或大小")
		}
		if size, ok := seen[hash]; ok {
			if size != a.Size {
				return nil, errors.New("资源索引中的文件大小冲突")
			}
			continue
		}
		seen[hash] = a.Size
		key := hash[:2] + "/" + hash
		obj.URL = "https://resources.download.minecraft.net/" + key
		files = append(files, fileDownload{filepath.Join(g.Cache, "assets", "objects", filepath.FromSlash(key)), obj})
	}
	return files, nil
}

func (g Game) Install(ctx context.Context, i Instance, report func(Progress)) error {
	return g.install(ctx, i, report, &downloads{http: officialHTTP})
}

func (g Game) install(ctx context.Context, i Instance, report func(Progress), d *downloads) error {
	if err := supported(i); err != nil {
		return err
	}
	if report == nil {
		report = func(Progress) {}
	}
	report(Progress{Stage: "获取官方版本元数据"})
	m, v, err := g.downloadVersion(ctx, i.Version, d)
	if err != nil {
		return err
	}
	dir := g.versionDir(i.Version)
	files, _, err := g.binaries(m)
	if err != nil {
		return err
	}
	if err := d.batch(ctx, "校验游戏和运行库", files, report); err != nil {
		return err
	}
	indexPath, err := g.indexPath(m)
	if err != nil {
		return err
	}
	if err := d.file(ctx, indexPath, m.AssetIndex.artifact); err != nil {
		return err
	}
	assets, err := g.assets(indexPath)
	if err != nil {
		return err
	}
	if err := d.batch(ctx, "校验游戏资源", assets, report); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	receipt, err := json.Marshal(installReceipt{Platform: platformKey(), VersionSHA: v.SHA})
	if err != nil {
		return err
	}
	return writeFile(filepath.Join(dir, "verified"), receipt)
}

func (g Game) downloadVersion(ctx context.Context, id string, d *downloads) (metadata, Version, error) {
	all, err := d.releases(ctx)
	if err != nil {
		return metadata{}, Version{}, err
	}
	index := slices.IndexFunc(all, func(v Version) bool { return v.ID == id })
	if index == -1 {
		return metadata{}, Version{}, fmt.Errorf("官方正式版本目录中没有 %s", id)
	}
	v := all[index]
	if err := d.file(ctx, filepath.Join(g.versionDir(id), "version.json"), artifact{URL: v.URL, SHA: v.SHA, Size: unknownSize}); err != nil {
		return metadata{}, Version{}, err
	}
	m, err := g.readVersion(id, v.SHA)
	return m, v, err
}

func verifyArtifact(ctx context.Context, file fileDownload) error {
	digest, err := file.file.checksum()
	if err != nil {
		return fmt.Errorf("%s：%w", filepath.Base(file.path), err)
	}
	ok, err := checkFile(ctx, file.path, file.file, digest)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%s 缺失或未通过校验", filepath.Base(file.path))
	}
	return nil
}

func (g Game) verifyInstalled(ctx context.Context, i Instance) (metadata, []string, error) {
	r, err := g.readReceipt(i.Version)
	if errors.Is(err, os.ErrNotExist) {
		err = errors.New("没有已校验的安装记录")
	}
	if err != nil {
		return metadata{}, nil, err
	}
	m, err := g.readVersion(i.Version, r.VersionSHA)
	if err != nil {
		return m, nil, err
	}
	files, cp, err := g.binaries(m)
	if err != nil {
		return m, nil, err
	}
	index, err := g.indexPath(m)
	if err != nil {
		return m, nil, err
	}
	// Check the asset index before trusting its references to other files.
	if err := verifyArtifact(ctx, fileDownload{index, m.AssetIndex.artifact}); err != nil {
		return m, nil, err
	}
	assets, err := g.assets(index)
	if err != nil {
		return m, nil, err
	}
	return m, cp, batch(ctx, append(files, assets...), verifyArtifact, nil)
}

type Running struct {
	PID  int
	Log  string
	Done <-chan error
}

func (g Game) Start(ctx context.Context, i Instance, settings Settings) (Running, error) {
	if err := supported(i); err != nil {
		return Running{}, err
	}
	if !playerID.MatchString(settings.Player) {
		return Running{}, errors.New("离线名称须为 3–16 位字母、数字或下划线")
	}
	if err := i.Launch.Validate(); err != nil {
		return Running{}, err
	}
	memory := i.Launch.MaxMemory(settings.Memory)
	if err := validateMemory(memory); err != nil {
		return Running{}, err
	}
	m, cp, err := g.verifyInstalled(ctx, i)
	if err != nil {
		if ctx.Err() != nil {
			return Running{}, ctx.Err()
		}
		return Running{}, fmt.Errorf("%w：%w", ErrRepairRequired, err)
	}
	for _, path := range cp {
		if strings.ContainsRune(path, os.PathListSeparator) {
			return Running{}, fmt.Errorf("缓存目录不能包含 Java 类路径分隔符 %q", os.PathListSeparator)
		}
	}
	j, err := FindJava(ctx, i.Launch.JavaExecutable(settings.JavaPath), m.JavaVersion.MajorVersion)
	if err != nil {
		if ctx.Err() != nil {
			return Running{}, ctx.Err()
		}
		return Running{}, fmt.Errorf("%w：%w", ErrJavaRequired, err)
	}
	if err := ctx.Err(); err != nil {
		return Running{}, err
	}
	natives := filepath.Join(i.GameDir, ".lxmcl-natives")
	if err := os.MkdirAll(natives, 0755); err != nil {
		return Running{}, fmt.Errorf("无法创建实例运行目录：%w", err)
	}
	uuid := md5.Sum([]byte("OfflinePlayer:" + settings.Player))
	uuid[6] = uuid[6]&0x0f | 0x30
	uuid[8] = uuid[8]&0x3f | 0x80
	values := map[string]string{
		"natives_directory": natives, "launcher_name": "LXMCL", "launcher_version": "0.2",
		"classpath": strings.Join(cp, string(os.PathListSeparator)), "classpath_separator": string(os.PathListSeparator),
		"library_directory": filepath.Join(g.Cache, "libraries"), "auth_player_name": settings.Player,
		"version_name": m.ID, "game_directory": i.GameDir, "assets_root": filepath.Join(g.Cache, "assets"),
		"assets_index_name": m.AssetIndex.ID, "auth_uuid": hex.EncodeToString(uuid[:]), "auth_access_token": "0",
		"clientid": "", "auth_xuid": "", "user_type": "legacy", "version_type": m.Type,
		"user_properties": "{}",
	}
	jvm, err := gameArgs(m.Arguments.JVM, values, nil)
	if err != nil {
		return Running{}, fmt.Errorf("无法生成 Java 参数：%w", err)
	}
	game, err := clientArgs(m, values, i.Launch)
	if err != nil {
		return Running{}, fmt.Errorf("无法生成游戏参数：%w", err)
	}
	args := append([]string{"-Xmx" + strconv.Itoa(memory) + "M"}, jvm...)
	if l := m.Logging.Client; l.Argument != "" {
		args = append(args, strings.ReplaceAll(l.Argument, "${path}", filepath.Join(g.Cache, "assets", "log_configs", l.File.ID)))
	}
	args = append(append(args, m.MainClass), game...)
	logDir := filepath.Join(i.GameDir, "launcher-logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return Running{}, fmt.Errorf("无法创建游戏日志目录：%w", err)
	}
	log, err := os.CreateTemp(logDir, "game-*.log")
	if err != nil {
		return Running{}, fmt.Errorf("无法创建游戏日志：%w", err)
	}
	if _, err := fmt.Fprintf(log, "[LXMCL] Minecraft %s · Java %d (%s) · -Xmx%dM\n", i.Version, j.Major, j.Path, memory); err != nil {
		log.Close()
		return Running{}, fmt.Errorf("无法写入游戏日志：%w", err)
	}
	// The game process owns its own lifetime after Start, independent of the launcher window.
	cmd := exec.Command(j.Path, args...)
	hideConsole(cmd)
	cmd.Dir, cmd.Stdout, cmd.Stderr = i.GameDir, log, log
	if err := ctx.Err(); err != nil {
		log.Close()
		return Running{}, err
	}
	if err := cmd.Start(); err != nil {
		log.Close()
		return Running{}, fmt.Errorf("启动 Java 进程失败：%w", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- errors.Join(cmd.Wait(), log.Close())
		close(done)
	}()
	return Running{PID: cmd.Process.Pid, Log: log.Name(), Done: done}, nil
}

func clientArgs(m metadata, values map[string]string, opts LaunchOptions) ([]string, error) {
	var features map[string]bool
	if opts.Display == DisplayResolution {
		features = map[string]bool{"has_custom_resolution": true}
		values["resolution_width"] = strconv.Itoa(opts.Width)
		values["resolution_height"] = strconv.Itoa(opts.Height)
	}
	args, err := gameArgs(m.Arguments.Game, values, features)
	if err != nil {
		return nil, err
	}
	if opts.Display == DisplayFullscreen && !slices.Contains(args, "--fullscreen") {
		args = append(args, "--fullscreen")
	}
	return args, nil
}

func ValidateSettings(s Settings) error {
	if !filepath.IsAbs(s.GameRoot) {
		return errors.New("游戏目录须为绝对路径")
	}
	if !playerID.MatchString(s.Player) {
		return errors.New("离线名称须为 3–16 位字母、数字或下划线")
	}
	return validateMemory(s.Memory)
}

func validateMemory(memory int) error {
	if memory < 1024 || memory > 65536 {
		return errors.New("最大内存须为 1024–65536 MiB")
	}
	return nil
}

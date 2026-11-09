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
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
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

var httpClient = &http.Client{
	Timeout: 2 * time.Minute,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !officialURL(req.URL.String()) {
			return errors.New("下载重定向超出官方来源")
		}
		return nil
	},
}

func officialURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	return slices.Contains([]string{"piston-meta.mojang.com", "piston-data.mojang.com",
		"libraries.minecraft.net", "resources.download.minecraft.net", "launcher.mojang.com",
		"launchermeta.mojang.com"}, u.Hostname())
}

func get(ctx context.Context, src string) (*http.Response, error) {
	if !officialURL(src) {
		return nil, fmt.Errorf("下载来源不受支持：%s", src)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err == nil && resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("下载返回 HTTP %d：%s", resp.StatusCode, src)
	}
	return resp, err
}

func Catalog(ctx context.Context) ([]Version, error) {
	resp, err := get(ctx, manifestURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var data struct{ Versions []Version }
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&data); err != nil {
		return nil, err
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
	all, err := Catalog(ctx)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(all, func(v Version) bool {
		return v.Type != "release" || !modernRelease(v.ID)
	}), nil
}

func allowed(rules []rule) bool {
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
		for _, want := range r.Features {
			if want {
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

func gameArgs(raw []json.RawMessage, values map[string]string) ([]string, error) {
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
		if !allowed(entry.Rules) {
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
		for _, m := range variable.FindAllStringSubmatch(s, -1) {
			value, ok := values[m[1]]
			if !ok {
				return nil, fmt.Errorf("不支持的启动参数 %s", m[0])
			}
			s = strings.ReplaceAll(s, m[0], value)
		}
		args[i] = s
	}
	return args, nil
}

func download(ctx context.Context, file string, a artifact) error {
	digest, err := hex.DecodeString(a.SHA)
	if err != nil || len(digest) != sha1.Size {
		return fmt.Errorf("文件缺少有效 SHA-1：%s", file)
	}
	if f, err := os.Open(file); err == nil {
		h := sha1.New()
		n, err := io.Copy(h, f)
		f.Close()
		if err == nil && (a.Size == 0 || n == a.Size) && hex.EncodeToString(h.Sum(nil)) == a.SHA {
			return nil
		}
	}
	resp, err := get(ctx, a.URL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(file), ".download-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	h := sha1.New()
	limit := a.Size + 1
	if a.Size == 0 {
		limit = 8 << 20
	}
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, limit))
	if err = errors.Join(err, f.Close()); err != nil {
		return err
	}
	if (a.Size > 0 && n != a.Size) || hex.EncodeToString(h.Sum(nil)) != a.SHA {
		return fmt.Errorf("文件校验失败：%s", filepath.Base(file))
	}
	return os.Rename(f.Name(), file)
}

type fileDownload struct {
	path string
	file artifact
}

func batch(ctx context.Context, stage string, files []fileDownload, report func(Progress)) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	report(Progress{Stage: stage, Total: len(files)})
	var wg sync.WaitGroup
	var next, done atomic.Int32
	var first error
	var once sync.Once
	for range min(12, len(files)) {
		wg.Go(func() {
			for ctx.Err() == nil {
				index := int(next.Add(1)) - 1
				if index >= len(files) {
					return
				}
				f := files[index]
				if err := download(ctx, f.path, f.file); err != nil {
					once.Do(func() { first = err; cancel() })
					return
				}
				report(Progress{Stage: stage, Done: int(done.Add(1)), Total: len(files)})
			}
		})
	}
	wg.Wait()
	if first != nil {
		return first
	}
	return ctx.Err()
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
	_, err := os.Stat(filepath.Join(g.versionDir(i.Version), "verified"))
	return err == nil
}

func (g Game) readVersion(id string) (metadata, error) {
	var m metadata
	found, err := readJSON(filepath.Join(g.versionDir(id), "version.json"), &m)
	if err == nil && (!found || m.ID != id || m.Type != "release" || m.MainClass == "" || m.JavaVersion.MajorVersion < 21) {
		err = errors.New("版本元数据缺失或不受支持")
	}
	return m, err
}

func (g Game) binaries(m metadata) ([]fileDownload, []string, error) {
	client := filepath.Join(g.versionDir(m.ID), "client.jar")
	files := []fileDownload{{client, m.Downloads.Client}}
	var cp []string
	for _, lib := range m.Libraries {
		if !allowed(lib.Rules) {
			continue
		}
		if len(lib.Natives) != 0 {
			return nil, nil, errors.New("当前版本使用不受支持的旧版本机库格式")
		}
		file := lib.Downloads.Artifact
		if file.Path == "" || !filepath.IsLocal(file.Path) {
			return nil, nil, fmt.Errorf("无效的库路径：%q", file.Path)
		}
		path := filepath.Join(g.Cache, "libraries", filepath.FromSlash(file.Path))
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
	seen := make(map[string]bool)
	for _, a := range assets.Objects {
		digest, err := hex.DecodeString(a.Hash)
		if err != nil || len(digest) != sha1.Size {
			return nil, errors.New("资源索引包含无效 SHA-1")
		}
		if seen[a.Hash] {
			continue
		}
		seen[a.Hash] = true
		key := a.Hash[:2] + "/" + a.Hash
		files = append(files, fileDownload{filepath.Join(g.Cache, "assets", "objects", filepath.FromSlash(key)),
			artifact{URL: "https://resources.download.minecraft.net/" + key, SHA: a.Hash, Size: a.Size}})
	}
	return files, nil
}

func (g Game) Install(ctx context.Context, i Instance, report func(Progress)) error {
	if err := supported(i); err != nil {
		return err
	}
	report(Progress{Stage: "获取官方版本元数据"})
	all, err := Releases(ctx)
	if err != nil {
		return err
	}
	index := slices.IndexFunc(all, func(v Version) bool { return v.ID == i.Version })
	if index == -1 {
		return fmt.Errorf("官方正式版本目录中没有 %s", i.Version)
	}
	v := all[index]
	dir := g.versionDir(i.Version)
	if err := download(ctx, filepath.Join(dir, "version.json"), artifact{URL: v.URL, SHA: v.SHA}); err != nil {
		return err
	}
	m, err := g.readVersion(i.Version)
	if err != nil {
		return err
	}
	files, _, err := g.binaries(m)
	if err != nil {
		return err
	}
	if err := batch(ctx, "校验游戏和运行库", files, report); err != nil {
		return err
	}
	indexPath, err := g.indexPath(m)
	if err != nil {
		return err
	}
	if err := download(ctx, indexPath, m.AssetIndex.artifact); err != nil {
		return err
	}
	assets, err := g.assets(indexPath)
	if err != nil {
		return err
	}
	if err := batch(ctx, "校验游戏资源", assets, report); err != nil {
		return err
	}
	return writeFile(filepath.Join(dir, "verified"), []byte("1\n"))
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
	if !g.Installed(i) {
		return Running{}, errors.New("请先安装游戏")
	}
	m, err := g.readVersion(i.Version)
	if err != nil {
		return Running{}, err
	}
	files, cp, err := g.binaries(m)
	if err != nil {
		return Running{}, err
	}
	index, err := g.indexPath(m)
	if err != nil {
		return Running{}, err
	}
	assets, err := g.assets(index)
	if err != nil {
		return Running{}, fmt.Errorf("%w，请重新校验安装", err)
	}
	for _, file := range append(append(files, fileDownload{path: index, file: m.AssetIndex.artifact}), assets...) {
		if err := ctx.Err(); err != nil {
			return Running{}, err
		}
		st, err := os.Stat(file.path)
		if err != nil || !st.Mode().IsRegular() || file.file.Size > 0 && file.file.Size != st.Size() {
			return Running{}, fmt.Errorf("%s 缺失或已变更，请重新校验安装", filepath.Base(file.path))
		}
	}
	j, err := FindJava(ctx, settings.JavaPath, m.JavaVersion.MajorVersion)
	if err != nil {
		return Running{}, err
	}
	natives := filepath.Join(i.GameDir, ".lxmcl-natives")
	if err := os.MkdirAll(natives, 0755); err != nil {
		return Running{}, err
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
	jvm, err := gameArgs(m.Arguments.JVM, values)
	if err != nil {
		return Running{}, err
	}
	game, err := gameArgs(m.Arguments.Game, values)
	if err != nil {
		return Running{}, err
	}
	args := append([]string{"-Xmx" + strconv.Itoa(settings.Memory) + "M"}, jvm...)
	if l := m.Logging.Client; l.Argument != "" {
		args = append(args, strings.ReplaceAll(l.Argument, "${path}", filepath.Join(g.Cache, "assets", "log_configs", l.File.ID)))
	}
	args = append(append(args, m.MainClass), game...)
	logDir := filepath.Join(i.GameDir, "launcher-logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return Running{}, err
	}
	log, err := os.CreateTemp(logDir, "game-*.log")
	if err != nil {
		return Running{}, err
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
		return Running{}, err
	}
	done := make(chan error, 1)
	go func() {
		done <- errors.Join(cmd.Wait(), log.Close())
		close(done)
	}()
	return Running{PID: cmd.Process.Pid, Log: log.Name(), Done: done}, nil
}

func ValidateSettings(s Settings) error {
	if !filepath.IsAbs(s.GameRoot) {
		return errors.New("游戏目录须为绝对路径")
	}
	if !playerID.MatchString(s.Player) {
		return errors.New("离线名称须为 3–16 位字母、数字或下划线")
	}
	if s.Memory < 1024 || s.Memory > 65536 {
		return errors.New("最大内存须为 1024–65536 MiB")
	}
	return nil
}

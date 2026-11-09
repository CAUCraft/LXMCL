package launcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

const fabricMeta = "https://meta.fabricmc.net/v2/versions/loader/"

var (
	fabricVersionID = regexp.MustCompile(`^[0-9][0-9A-Za-z.+_-]{0,63}$`)
	mavenPart       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.+_-]{0,127}$`)
)

func (i Instance) Vanilla() bool { return i.Loader == "" || strings.EqualFold(i.Loader, "vanilla") }
func (i Instance) Fabric() bool  { return strings.EqualFold(i.Loader, "fabric") }

func downloadJSON(ctx context.Context, d *downloads, source string, out any) error {
	return retry(ctx, func() error {
		res, err := d.get(ctx, source)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		body, err := io.ReadAll(io.LimitReader(res.Body, maxMetadata+1))
		if err != nil {
			return err
		}
		if len(body) > maxMetadata {
			return errors.New("服务返回的元数据过大")
		}
		return json.Unmarshal(body, out)
	})
}

func FabricVersions(ctx context.Context, gameVersion string) ([]string, error) {
	if !modernRelease(gameVersion) {
		return nil, errors.New("请选择受支持的 Minecraft 正式版本")
	}
	var entries []struct {
		Loader struct {
			Version string
			Stable  bool
		}
	}
	if err := downloadJSON(ctx, &downloads{http: officialHTTP}, fabricMeta+url.PathEscape(gameVersion), &entries); err != nil {
		return nil, err
	}
	var stable []string
	for _, entry := range entries {
		if entry.Loader.Stable && fabricVersionID.MatchString(entry.Loader.Version) {
			stable = append(stable, entry.Loader.Version)
		}
	}
	return stable, nil
}

type fabricArguments struct{ JVM, Game []json.RawMessage }

type fabricRuntime struct {
	GameVersion   string
	LoaderVersion string
	MainClass     string
	Arguments     fabricArguments
	Libraries     []artifact
}

func (g Game) fabricDir(i Instance) string {
	return filepath.Join(g.Cache, "fabric", i.Version, i.LoaderVersion)
}

func (g Game) readFabric(i Instance) (fabricRuntime, error) {
	var profile fabricRuntime
	ok, err := readJSON(filepath.Join(g.fabricDir(i), "verified.json"), &profile)
	if err != nil {
		return profile, err
	}
	if !ok || profile.GameVersion != i.Version || profile.LoaderVersion != i.LoaderVersion ||
		profile.MainClass != "net.fabricmc.loader.impl.launch.knot.KnotClient" || len(profile.Libraries) == 0 {
		return profile, errors.New("Fabric 文件尚未校验")
	}
	return profile, nil
}

func fabricLibraryPath(name string) (string, error) {
	parts := strings.Split(name, ":")
	if len(parts) != 3 || !mavenPart.MatchString(parts[1]) || !mavenPart.MatchString(parts[2]) {
		return "", fmt.Errorf("不支持的 Fabric 运行库：%s", name)
	}
	group := strings.Split(parts[0], ".")
	for _, part := range group {
		if !mavenPart.MatchString(part) {
			return "", fmt.Errorf("无效的 Fabric 运行库坐标：%s", name)
		}
	}
	return strings.Join(group, "/") + "/" + parts[1] + "/" + parts[2] + "/" + parts[1] + "-" + parts[2] + ".jar", nil
}

func (g Game) fabricFiles(i Instance, libs []artifact) ([]fileDownload, []string, error) {
	var files []fileDownload
	var classpath []string
	for _, lib := range libs {
		if !filepath.IsLocal(lib.Path) || strings.ContainsAny(lib.Path, ":\\") || filepath.Ext(lib.Path) != ".jar" {
			return nil, nil, fmt.Errorf("无效的 Fabric 运行库路径：%s", lib.Path)
		}
		path := filepath.Join(g.fabricDir(i), "libraries", filepath.FromSlash(lib.Path))
		files = append(files, fileDownload{path, lib})
		classpath = append(classpath, path)
	}
	return files, classpath, nil
}

func (g Game) installFabric(ctx context.Context, i Instance, d *downloads, report func(Progress)) error {
	report(Progress{Stage: "获取 Fabric 加载器"})
	var remote struct {
		ID           string
		InheritsFrom string
		MainClass    string
		Arguments    fabricArguments
		Libraries    []struct {
			Name, URL, SHA1 string
			Size            int64
		}
	}
	source := fabricMeta + url.PathEscape(i.Version) + "/" + url.PathEscape(i.LoaderVersion) + "/profile/json"
	if err := downloadJSON(ctx, d, source, &remote); err != nil {
		return err
	}
	if remote.InheritsFrom != i.Version || remote.ID != "fabric-loader-"+i.LoaderVersion+"-"+i.Version ||
		remote.MainClass != "net.fabricmc.loader.impl.launch.knot.KnotClient" || len(remote.Libraries) == 0 {
		return errors.New("Fabric 配置不匹配当前 Minecraft 与加载器版本")
	}
	p := fabricRuntime{GameVersion: i.Version, LoaderVersion: i.LoaderVersion, MainClass: remote.MainClass, Arguments: remote.Arguments}
	for _, lib := range remote.Libraries {
		path, err := fabricLibraryPath(lib.Name)
		if err != nil {
			return err
		}
		if lib.URL != "https://maven.fabricmc.net/" && lib.URL != "https://libraries.minecraft.net/" {
			return fmt.Errorf("Fabric 运行库来源不受支持：%s", lib.URL)
		}
		a := artifact{Path: path, URL: lib.URL + path, SHA: lib.SHA1, Size: lib.Size}
		if a.Size == 0 {
			a.Size = unknownSize
		}
		if a.SHA == "" {
			err := retry(ctx, func() error {
				res, err := d.get(ctx, a.URL+".sha1")
				if err != nil {
					return err
				}
				defer res.Body.Close()
				b, err := io.ReadAll(io.LimitReader(res.Body, 128))
				a.SHA = strings.TrimSpace(string(b))
				return err
			})
			if err != nil {
				return err
			}
		}
		if _, err := a.checksum(); err != nil {
			return err
		}
		p.Libraries = append(p.Libraries, a)
	}
	files, _, err := g.fabricFiles(i, p.Libraries)
	if err != nil {
		return err
	}
	if err := d.batch(ctx, "下载并校验 Fabric", files, report); err != nil {
		return err
	}
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return writeFile(filepath.Join(g.fabricDir(i), "verified.json"), data)
}

func (g Game) verifyFabric(ctx context.Context, i Instance) (fabricRuntime, []string, error) {
	p, err := g.readFabric(i)
	if err != nil {
		return p, nil, err
	}
	files, cp, err := g.fabricFiles(i, p.Libraries)
	if err == nil {
		err = batch(ctx, files, func(ctx context.Context, f fileDownload) error { return verifyArtifact(ctx, f) }, nil)
	}
	return p, cp, err
}

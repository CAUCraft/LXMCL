package launcher

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

const modrinthAPI = "https://api.modrinth.com/v2/"

var modrinthID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type PackProject struct {
	ID     string `json:"project_id"`
	Title  string `json:"title"`
	Author string `json:"author"`
}

type PackVersion struct {
	ID           string   `json:"id"`
	ProjectID    string   `json:"project_id"`
	Number       string   `json:"version_number"`
	Type         string   `json:"version_type"`
	GameVersions []string `json:"game_versions"`
	Loaders      []string `json:"loaders"`
	Files        []PackFile
	Dependencies []ModDependency
}

type ModDependency struct {
	ProjectID string `json:"project_id"`
	VersionID string `json:"version_id"`
	Type      string `json:"dependency_type"`
}

type PackFile struct {
	URL, Filename string
	Size          int64
	Primary       bool
	Hashes        struct{ SHA512 string }
}

func SearchModrinth(ctx context.Context, game string, kind ContentKind, query string) ([]PackProject, error) {
	if !modernRelease(game) {
		return nil, errors.New("请选择受支持的 Minecraft 正式版本")
	}
	filter := [][]string{{"project_type:resourcepack"}, {"versions:" + game}}
	if kind == FabricMods {
		filter[0][0] = "project_type:mod"
		filter = append(filter, []string{"categories:fabric"})
	}
	facets, _ := json.Marshal(filter)
	params := url.Values{"query": {strings.TrimSpace(query)}, "facets": {string(facets)}, "limit": {"15"}}
	var result struct{ Hits []PackProject }
	err := downloadJSON(ctx, &downloads{http: officialHTTP}, modrinthAPI+"search?"+params.Encode(), &result)
	return result.Hits, err
}

func ModrinthVersions(ctx context.Context, project, game string, kind ContentKind) ([]PackVersion, error) {
	return modrinthVersions(ctx, project, game, kind, &downloads{http: officialHTTP})
}

func modrinthVersions(ctx context.Context, project, game string, kind ContentKind, d *downloads) ([]PackVersion, error) {
	if !modrinthID.MatchString(project) || !modernRelease(game) {
		return nil, errors.New("资源项目或 Minecraft 版本无效")
	}
	versions, _ := json.Marshal([]string{game})
	params := url.Values{"game_versions": {string(versions)}, "include_changelog": {"false"}}
	if kind == FabricMods {
		params.Set("loaders", `["fabric"]`)
	}
	var result []PackVersion
	err := downloadJSON(ctx, d, modrinthAPI+"project/"+project+"/version?"+params.Encode(), &result)
	return result, err
}

func DownloadModrinth(ctx context.Context, i Instance, kind ContentKind, version PackVersion) error {
	return downloadModrinth(ctx, i, kind, version, &downloads{http: officialHTTP})
}

func downloadModrinth(ctx context.Context, i Instance, kind ContentKind, version PackVersion, d *downloads) error {
	if _, _, err := kind.format(i); err != nil {
		return err
	}
	if kind != FabricMods {
		return downloadModrinthFile(ctx, i, kind, version, d)
	}
	plan, err := fabricModPlan(ctx, i.Version, version, d)
	if err != nil {
		return err
	}
	for n, v := range plan {
		if err := downloadModrinthFile(ctx, i, kind, v, d); err != nil {
			return fmt.Errorf("已就绪 %d/%d 个模组文件：%w", n, len(plan), err)
		}
	}
	return nil
}

func (v PackVersion) fabricCompatible(game string) bool {
	return slices.Contains(v.GameVersions, game) && slices.Contains(v.Loaders, "fabric")
}

func fabricModPlan(ctx context.Context, game string, primary PackVersion, d *downloads) ([]PackVersion, error) {
	chosen := make(map[string]string)
	var plan []PackVersion
	var add func(PackVersion) error
	add = func(v PackVersion) error {
		if !modrinthID.MatchString(v.ProjectID) || !modrinthID.MatchString(v.ID) || !v.fabricCompatible(game) {
			return errors.New("模组或必要依赖不兼容此 Minecraft / Fabric 版本")
		}
		if prior, ok := chosen[v.ProjectID]; ok {
			if prior != v.ID {
				return fmt.Errorf("模组依赖需要同一项目的不同版本：%s", v.ProjectID)
			}
			return nil
		}
		if len(chosen) >= 32 {
			return errors.New("模组依赖超过 32 项，请分开安装")
		}
		chosen[v.ProjectID] = v.ID
		for _, dep := range v.Dependencies {
			if dep.Type != "required" {
				continue
			}
			if dep.VersionID == "" && chosen[dep.ProjectID] != "" {
				continue
			}
			resolved, err := resolveModDependency(ctx, game, dep, d)
			if err != nil {
				return err
			}
			if err := add(resolved); err != nil {
				return err
			}
		}
		plan = append(plan, v)
		return nil
	}
	if err := add(primary); err != nil {
		return nil, err
	}
	return plan, nil
}

func resolveModDependency(ctx context.Context, game string, dep ModDependency, d *downloads) (PackVersion, error) {
	var result PackVersion
	if dep.VersionID != "" {
		if !modrinthID.MatchString(dep.VersionID) {
			return result, errors.New("模组依赖版本无效")
		}
		if err := downloadJSON(ctx, d, modrinthAPI+"version/"+dep.VersionID, &result); err != nil {
			return result, err
		}
		if result.ID != dep.VersionID || dep.ProjectID != "" && result.ProjectID != dep.ProjectID {
			return result, errors.New("模组依赖版本与项目不符")
		}
		return result, nil
	}
	if !modrinthID.MatchString(dep.ProjectID) {
		return result, errors.New("此模组有无法自动解析的必要依赖")
	}
	versions, err := modrinthVersions(ctx, dep.ProjectID, game, FabricMods, d)
	if err != nil {
		return result, err
	}
	for _, v := range versions {
		if v.ProjectID == dep.ProjectID && v.fabricCompatible(game) {
			if v.Type == "release" {
				return v, nil
			}
			if result.ID == "" {
				result = v
			}
		}
	}
	return result, nil
}

func downloadModrinthFile(ctx context.Context, i Instance, kind ContentKind, v PackVersion, d *downloads) error {
	if !slices.Contains(v.GameVersions, i.Version) || kind == FabricMods && !v.fabricCompatible(i.Version) {
		return errors.New("此资源版本与当前实例不兼容")
	}
	ext, _, err := kind.format(i)
	if err != nil {
		return err
	}
	valid := func(f PackFile) bool { return contentName(f.Filename, ext) }
	n := slices.IndexFunc(v.Files, func(f PackFile) bool { return f.Primary && valid(f) })
	if n == -1 {
		n = slices.IndexFunc(v.Files, valid)
	}
	if n == -1 {
		return fmt.Errorf("此版本没有 %s 文件", ext)
	}
	file := v.Files[n]
	u, err := url.Parse(file.URL)
	if err != nil || u.Hostname() != "cdn.modrinth.com" || !officialURL(u) {
		return errors.New("资源下载来源不受支持")
	}
	if digest, err := hex.DecodeString(file.Hashes.SHA512); err != nil || len(digest) != sha512.Size {
		return errors.New("资源缺少有效的 SHA-512")
	}
	if file.Size <= 0 || file.Size > maxContentSize {
		return errors.New("资源文件大小不受支持")
	}
	existing, err := alreadyDownloaded(ctx, i, kind, file)
	if err != nil || existing {
		return err
	}
	return retry(ctx, func() error {
		res, err := d.get(ctx, file.URL)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		return writeContent(ctx, i, kind, file.Filename, res.Body, file.Size, sha512.New(), file.Hashes.SHA512)
	})
}

func alreadyDownloaded(ctx context.Context, i Instance, kind ContentKind, file PackFile) (bool, error) {
	root, err := contentDirectory(i, kind, false)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer root.Close()
	info, err := root.Lstat(file.Filename)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() || info.Size() != file.Size {
		return false, fmt.Errorf("已有同名文件：%s", file.Filename)
	}
	src, err := root.Open(file.Filename)
	if err != nil {
		return false, err
	}
	defer src.Close()
	h := sha512.New()
	n, err := io.Copy(h, io.LimitReader(cancelReader{ctx: ctx, r: src}, file.Size+1))
	if err != nil {
		return false, err
	}
	if n != file.Size || !strings.EqualFold(fmt.Sprintf("%x", h.Sum(nil)), file.Hashes.SHA512) {
		return false, fmt.Errorf("已有同名文件，内容不同：%s", file.Filename)
	}
	return true, nil
}

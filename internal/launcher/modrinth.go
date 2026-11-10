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
	"strconv"
	"strings"
	"time"
)

const modrinthAPI = "https://api.modrinth.com/v2/"

const (
	fabricAPIProject  = "P7dR8mSH"
	quiltedAPIProject = "qvIfYCYJ"
)

var modrinthID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type PackProject struct {
	ID          string `json:"project_id"`
	Title       string `json:"title"`
	Author      string `json:"author"`
	Description string `json:"description"`
	Downloads   int    `json:"downloads"`
	IconURL     string `json:"icon_url"`
}

// ResourceFilter describes a catalog query independently of an installation.
type ResourceFilter struct{ Game, Loader string }

func (f ResourceFilter) valid(kind ContentKind) bool {
	return (f.Game == "" || releaseID.MatchString(f.Game)) && kind.modrinthType() != "" &&
		(f.Loader == "" || kind == Mods && slices.Contains([]string{"fabric", "forge", "neoforge", "quilt"}, f.Loader))
}

func (f ResourceFilter) matches(v PackVersion) bool {
	return (f.Game == "" || slices.Contains(v.GameVersions, f.Game)) &&
		(f.Loader == "" || slices.Contains(v.Loaders, f.Loader))
}

func instanceResourceFilter(i Instance, kind ContentKind) ResourceFilter {
	f := ResourceFilter{Game: i.Version}
	if kind == Mods {
		f.Loader = i.modrinthModLoader()
	}
	return f
}

func (p PackProject) PageURL(kind ContentKind) string {
	if !modrinthID.MatchString(p.ID) {
		return ""
	}
	if projectType := kind.modrinthType(); projectType != "" {
		return "https://modrinth.com/" + projectType + "/" + p.ID
	}
	return ""
}

type PackVersion struct {
	ID           string    `json:"id"`
	ProjectID    string    `json:"project_id"`
	Number       string    `json:"version_number"`
	Type         string    `json:"version_type"`
	GameVersions []string  `json:"game_versions"`
	Loaders      []string  `json:"loaders"`
	Published    time.Time `json:"date_published"`
	Files        []PackFile
	Dependencies []ModDependency
}

type ModDependency struct {
	ProjectID string `json:"project_id"`
	VersionID string `json:"version_id"`
	Filename  string `json:"file_name"`
	Type      string `json:"dependency_type"`
}

type PackFile struct {
	URL, Filename string
	Size          int64
	Primary       bool
	Hashes        struct{ SHA512 string }
}

const PackPageSize = 20

type PackSearch struct {
	Hits  []PackProject
	Total int `json:"total_hits"`
}

func (k ContentKind) modrinthType() string {
	switch k {
	case Mods:
		return "mod"
	case ResourcePacks:
		return "resourcepack"
	case ShaderPacks:
		return "shader"
	}
	return ""
}

func (i Instance) modrinthModLoader() string {
	if i.knot() || i.patched() {
		return strings.ToLower(i.Loader)
	}
	return ""
}

func SearchModrinth(ctx context.Context, i Instance, kind ContentKind, query, order string, page int) (PackSearch, error) {
	return searchModrinth(ctx, i, kind, query, order, page, &downloads{http: officialHTTP})
}

func searchModrinth(ctx context.Context, i Instance, kind ContentKind, query, order string, page int, d *downloads) (PackSearch, error) {
	if !modernRelease(i.Version) {
		return PackSearch{}, errors.New("请选择受支持的 Minecraft 正式版本")
	}
	if _, _, err := kind.format(i); err != nil {
		return PackSearch{}, err
	}
	return searchResourceCatalog(ctx, instanceResourceFilter(i, kind), kind, query, order, page, d)
}

func SearchResourceCatalog(ctx context.Context, filter ResourceFilter, kind ContentKind, query, order string, page int) (PackSearch, error) {
	return searchResourceCatalog(ctx, filter, kind, query, order, page, &downloads{http: officialHTTP})
}

func searchResourceCatalog(ctx context.Context, f ResourceFilter, kind ContentKind, query, order string, page int, d *downloads) (PackSearch, error) {
	projectType := kind.modrinthType()
	if !f.valid(kind) || page < 0 || page > 500 || len(query) > 512 {
		return PackSearch{}, errors.New("资源搜索条件无效")
	}
	if order != "downloads" && order != "updated" {
		order = "relevance"
	}
	filter := [][]string{{"project_type:" + projectType}}
	if f.Game != "" {
		filter = append(filter, []string{"versions:" + f.Game})
	}
	if f.Loader != "" {
		filter = append(filter, []string{"categories:" + f.Loader})
	}
	facets, _ := json.Marshal(filter)
	params := url.Values{"query": {strings.TrimSpace(query)}, "facets": {string(facets)}, "index": {order},
		"limit": {strconv.Itoa(PackPageSize)}, "offset": {strconv.Itoa(PackPageSize * page)}}
	var result PackSearch
	err := downloadJSON(ctx, d, modrinthAPI+"search?"+params.Encode(), &result)
	result.Hits = slices.DeleteFunc(result.Hits, func(p PackProject) bool {
		return !modrinthID.MatchString(p.ID) || p.Title == ""
	})
	return result, err
}

func ModrinthVersions(ctx context.Context, project string, i Instance, kind ContentKind) ([]PackVersion, error) {
	return modrinthVersions(ctx, project, i, kind, &downloads{http: officialHTTP})
}

func modrinthVersions(ctx context.Context, project string, i Instance, kind ContentKind, d *downloads) ([]PackVersion, error) {
	if !modrinthID.MatchString(project) || !modernRelease(i.Version) {
		return nil, errors.New("资源项目或 Minecraft 版本无效")
	}
	if _, _, err := kind.format(i); err != nil {
		return nil, err
	}
	return resourceCatalogVersions(ctx, project, instanceResourceFilter(i, kind), kind, d)
}

func ResourceCatalogVersions(ctx context.Context, project string, filter ResourceFilter, kind ContentKind) ([]PackVersion, error) {
	return resourceCatalogVersions(ctx, project, filter, kind, &downloads{http: officialHTTP})
}

func resourceCatalogVersions(ctx context.Context, project string, f ResourceFilter, kind ContentKind, d *downloads) ([]PackVersion, error) {
	if !modrinthID.MatchString(project) || !f.valid(kind) {
		return nil, errors.New("资源项目或筛选条件无效")
	}
	params := url.Values{"include_changelog": {"false"}}
	if f.Game != "" {
		versions, _ := json.Marshal([]string{f.Game})
		params.Set("game_versions", string(versions))
	}
	if f.Loader != "" {
		loaders, _ := json.Marshal([]string{f.Loader})
		params.Set("loaders", string(loaders))
	}
	var result []PackVersion
	err := downloadJSON(ctx, d, modrinthAPI+"project/"+project+"/version?"+params.Encode(), &result)
	return slices.DeleteFunc(result, func(v PackVersion) bool {
		_, ok := v.DownloadFile(kind)
		return v.ProjectID != project || !modrinthID.MatchString(v.ID) || !f.matches(v) || !ok
	}), err
}

func ModrinthVersion(ctx context.Context, project, id string) (PackVersion, error) {
	if !modrinthID.MatchString(project) {
		return PackVersion{}, errors.New("资源项目无效")
	}
	return modrinthVersion(ctx, project, id, &downloads{http: officialHTTP})
}

func modrinthVersion(ctx context.Context, project, id string, d *downloads) (PackVersion, error) {
	if (project != "" && !modrinthID.MatchString(project)) || !modrinthID.MatchString(id) {
		return PackVersion{}, errors.New("资源项目或版本无效")
	}
	var v PackVersion
	if err := downloadJSON(ctx, d, modrinthAPI+"version/"+id, &v); err != nil {
		return PackVersion{}, err
	}
	if v.ID != id || !modrinthID.MatchString(v.ProjectID) || project != "" && v.ProjectID != project {
		return PackVersion{}, errors.New("资源版本与项目不符")
	}
	return v, nil
}

func DownloadModrinth(ctx context.Context, i Instance, kind ContentKind, version PackVersion, report func(Progress)) error {
	return downloadModrinth(ctx, i, kind, version, &downloads{http: officialHTTP}, report)
}

func downloadModrinth(ctx context.Context, i Instance, kind ContentKind, version PackVersion, d *downloads, report func(Progress)) error {
	if _, _, err := kind.format(i); err != nil {
		return err
	}
	if kind == Mods && (!modrinthID.MatchString(version.ProjectID) || !modrinthID.MatchString(version.ID)) {
		return errors.New("模组项目或版本无效")
	}
	if report == nil {
		report = func(Progress) {}
	}
	const stage = "下载并校验资源"
	report(Progress{Stage: stage, Total: 1})
	if err := downloadModrinthFile(ctx, i, kind, version, d); err != nil {
		return err
	}
	report(Progress{Stage: stage, Done: 1, Total: 1})
	return nil
}

func (v PackVersion) compatible(i Instance, kind ContentKind) bool {
	if !slices.Contains(v.GameVersions, i.Version) {
		return false
	}
	return kind != Mods || slices.Contains(v.Loaders, i.modrinthModLoader()) ||
		i.Quilt() && v.ProjectID == fabricAPIProject && slices.Contains(v.Loaders, "fabric")
}

func (v PackVersion) DownloadFile(kind ContentKind) (PackFile, bool) {
	ext := ".zip"
	if kind == Mods {
		ext = ".jar"
	}
	valid := func(f PackFile) bool { return contentName(f.Filename, ext) }
	n := slices.IndexFunc(v.Files, func(f PackFile) bool { return f.Primary && valid(f) })
	if n == -1 {
		n = slices.IndexFunc(v.Files, valid)
	}
	if n == -1 {
		return PackFile{}, false
	}
	return v.Files[n], true
}

func downloadModrinthFile(ctx context.Context, i Instance, kind ContentKind, v PackVersion, d *downloads) error {
	if !v.compatible(i, kind) {
		return errors.New("此资源版本与当前实例不兼容")
	}
	file, ok := v.DownloadFile(kind)
	if !ok {
		return errors.New("此版本没有可下载的资源文件")
	}
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

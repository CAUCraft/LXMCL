package launcher

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
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
	return (f.Game == "" || instance.IsReleaseID(f.Game)) && kind.modrinthType() != "" &&
		(f.Loader == "" || kind == Mods && slices.Contains([]string{"fabric", "forge", "neoforge", "quilt"}, f.Loader))
}

func (f ResourceFilter) matches(v PackVersion) bool {
	return (f.Game == "" || slices.Contains(v.GameVersions, f.Game)) &&
		(f.Loader == "" || slices.Contains(v.Loaders, f.Loader))
}

func instanceResourceFilter(i Instance, kind ContentKind) ResourceFilter {
	f := ResourceFilter{Game: i.Version}
	if kind == Mods {
		f.Loader = modrinthModLoader(i)
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

func modrinthModLoader(i Instance) string {
	if i.UsesKnot() || i.UsesInstaller() {
		return strings.ToLower(i.Loader)
	}
	return ""
}

func SearchModrinth(ctx context.Context, i Instance, kind ContentKind, query, order string, page int) (PackSearch, error) {
	return searchModrinth(ctx, i, kind, query, order, page, &downloads{http: officialHTTP})
}

func searchModrinth(ctx context.Context, i Instance, kind ContentKind, query, order string, page int, d *downloads) (PackSearch, error) {
	if !instance.SupportedRelease(i.Version) {
		return PackSearch{}, i18n.Error("version.select_supported")
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
		return PackSearch{}, i18n.Error("modrinth.search_invalid")
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
	if !modrinthID.MatchString(project) || !instance.SupportedRelease(i.Version) {
		return nil, i18n.Error("modrinth.project_or_game_invalid")
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
		return nil, i18n.Error("modrinth.project_or_filter_invalid")
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
		return PackVersion{}, i18n.Error("modrinth.project_invalid")
	}
	return modrinthVersion(ctx, project, id, &downloads{http: officialHTTP})
}

func modrinthVersion(ctx context.Context, project, id string, d *downloads) (PackVersion, error) {
	if (project != "" && !modrinthID.MatchString(project)) || !modrinthID.MatchString(id) {
		return PackVersion{}, i18n.Error("modrinth.project_or_version_invalid")
	}
	var v PackVersion
	if err := downloadJSON(ctx, d, modrinthAPI+"version/"+id, &v); err != nil {
		return PackVersion{}, err
	}
	if v.ID != id || !modrinthID.MatchString(v.ProjectID) || project != "" && v.ProjectID != project {
		return PackVersion{}, i18n.Error("modrinth.version_project_mismatch")
	}
	return v, nil
}

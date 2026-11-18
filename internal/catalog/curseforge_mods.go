package catalog

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/i18n"
)

// CurseMods reads Minecraft mod projects and files through the official API.
type CurseMods struct{ api curseForge }

func (c Client) CurseMods(key string) CurseMods { return CurseMods{api: c.curseForge(key)} }

func curseModID(id string) int {
	n, err := strconv.ParseInt(id, 10, 32)
	if err != nil || n <= 0 || strconv.FormatInt(n, 10) != id {
		return 0
	}
	return int(n)
}

func curseModParams(filter ResourceFilter, page int) (url.Values, error) {
	if !filter.valid(content.Mods) || page < 0 || page >= 10000/PackPageSize {
		return nil, i18n.Error("curseforge.filter_invalid")
	}
	p := curseParams(filter.Game, page)
	if filter.Loader != "" {
		p.Set("modLoaderType", map[string]string{"forge": "1", "fabric": "4", "quilt": "5", "neoforge": "6"}[filter.Loader])
	}
	return p, nil
}

func (m curseProject) modProject() (PackProject, bool) {
	if m.ID <= 0 || m.ClassID != 6 || m.GameID != curseGameID || !m.IsAvailable || strings.TrimSpace(m.Name) == "" {
		return PackProject{}, false
	}
	p := PackProject{ID: strconv.Itoa(m.ID), Title: m.Name, Description: m.Summary, Downloads: m.DownloadCount, IconURL: m.Logo.ThumbnailURL}
	if p.IconURL == "" {
		p.IconURL = m.Logo.URL
	}
	if len(m.Authors) != 0 {
		p.Author = m.Authors[0].Name
	}
	p.cursePageURL = "https://www.curseforge.com/projects/" + p.ID
	if modrinthID.MatchString(m.Slug) {
		p.cursePageURL = "https://www.curseforge.com/minecraft/mc-mods/" + m.Slug
	}
	return p, true
}

func (c CurseMods) Search(ctx context.Context, filter ResourceFilter, query, order string, page int) (PackSearch, error) {
	p, err := curseModParams(filter, page)
	if err != nil {
		return PackSearch{}, err
	}
	if len(query) > 512 {
		return PackSearch{}, i18n.Error("curseforge.filter_invalid")
	}
	p.Set("gameId", strconv.Itoa(curseGameID))
	p.Set("classId", "6")
	p.Set("searchFilter", strings.TrimSpace(query))
	sort := map[string]string{"downloads": "6", "updated": "3", "popularity": "2"}[order]
	if sort != "" {
		p.Set("sortField", sort)
		p.Set("sortOrder", "desc")
	}
	var data cursePage[curseProject]
	if err := c.api.call(ctx, "mods/search?"+p.Encode(), nil, &data); err != nil {
		return PackSearch{}, err
	}
	result := PackSearch{Total: min(10000, data.Pagination.TotalCount)}
	for _, m := range data.Data {
		if project, ok := m.modProject(); ok {
			result.Hits = append(result.Hits, project)
		}
	}
	return result, nil
}

func (c CurseMods) project(ctx context.Context, id string) (PackProject, error) {
	n := curseModID(id)
	if n == 0 {
		return PackProject{}, i18n.Error("curseforge.project_id_invalid")
	}
	var reply struct{ Data curseProject }
	if err := c.api.call(ctx, "mods/"+id, nil, &reply); err != nil {
		return PackProject{}, err
	}
	p, ok := reply.Data.modProject()
	if !ok || reply.Data.ID != n {
		return PackProject{}, i18n.Error("curseforge.mod_unavailable")
	}
	if allowed := reply.Data.AllowModDistribution; allowed != nil && !*allowed {
		return p, i18n.Error("curseforge.mod_distribution_disabled")
	}
	return p, nil
}

func (f curseFile) modVersion(project int) (PackVersion, bool) {
	a := f.asset()
	if f.ID <= 0 || f.ModID != project || f.GameID != curseGameID || !f.IsAvailable || f.IsServerPack || f.IsEarlyAccessContent || !content.ValidName(f.FileName, ".jar") || !a.Valid(content.MaxSize) {
		return PackVersion{}, false
	}
	v := PackVersion{ID: strconv.Itoa(f.ID), ProjectID: strconv.Itoa(project), Number: f.DisplayName, Type: map[int]string{1: "release", 2: "beta", 3: "alpha"}[f.ReleaseType], Published: f.FileDate, curseAsset: a,
		Files: []PackFile{{URL: f.DownloadURL, Filename: f.FileName, Size: f.FileLength, Primary: true}}}
	if v.Number == "" {
		v.Number = f.FileName
	}
	for _, tag := range f.GameVersions {
		switch lower := strings.ToLower(tag); lower {
		case "forge", "fabric", "quilt", "neoforge":
			v.Loaders = append(v.Loaders, lower)
		default:
			if versionTag.MatchString(tag) {
				v.GameVersions = append(v.GameVersions, tag)
			}
		}
	}
	for _, d := range f.Dependencies {
		if d.RelationType == 3 && d.ModID > 0 {
			v.Dependencies = append(v.Dependencies, ModDependency{ProjectID: strconv.Itoa(d.ModID), Type: "required"})
		}
	}
	return v, true
}

func (c CurseMods) Files(ctx context.Context, id string, filter ResourceFilter, page int) (PackProject, []PackVersion, bool, error) {
	p, err := curseModParams(filter, page)
	if err != nil {
		return PackProject{}, nil, false, err
	}
	project, err := c.project(ctx, id)
	if err != nil {
		return project, nil, false, err
	}
	var data cursePage[curseFile]
	if err := c.api.call(ctx, "mods/"+id+"/files?"+p.Encode(), nil, &data); err != nil {
		return project, nil, false, err
	}
	var versions []PackVersion
	for _, f := range data.Data {
		if v, ok := f.modVersion(curseModID(id)); ok && filter.matches(v) {
			versions = append(versions, v)
		}
	}
	next := data.Pagination.Index + data.Pagination.ResultCount
	return project, versions, data.Pagination.ResultCount > 0 && next < min(10000, data.Pagination.TotalCount), nil
}

// Version rechecks distribution permission and the exact file before a queued download or retry.
func (c CurseMods) Version(ctx context.Context, project, file string) (PackVersion, error) {
	id := curseModID(file)
	if id == 0 {
		return PackVersion{}, i18n.Error("curseforge.file_unavailable")
	}
	if _, err := c.project(ctx, project); err != nil {
		return PackVersion{}, err
	}
	var data struct{ Data curseFile }
	if err := c.api.call(ctx, "mods/"+project+"/files/"+file, nil, &data); err != nil {
		return PackVersion{}, err
	}
	v, ok := data.Data.modVersion(curseModID(project))
	if !ok || data.Data.ID != id {
		return PackVersion{}, i18n.Error("curseforge.file_unavailable")
	}
	return v, nil
}

func (c CurseMods) DependencyProjects(ctx context.Context, deps []ModDependency) (map[string]PackProject, error) {
	ids := make([]int, 0, len(deps))
	for _, d := range deps {
		if id := curseModID(d.ProjectID); id > 0 {
			ids = append(ids, id)
		}
	}
	ids = uniquePackIDs(ids)
	result := make(map[string]PackProject)
	if len(ids) == 0 {
		return result, nil
	}
	var data cursePage[curseProject]
	if err := c.api.call(ctx, "mods", map[string][]int{"modIds": ids[:min(len(ids), 50)]}, &data); err != nil {
		return nil, err
	}
	for _, m := range data.Data {
		if p, ok := m.modProject(); ok {
			result[p.ID] = p
		}
	}
	return result, nil
}

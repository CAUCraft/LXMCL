package launcher

import (
	"context"
	"encoding/json"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

type ModpackSource string

const (
	ModrinthModpacks   ModpackSource = "Modrinth"
	CurseForgeModpacks ModpackSource = "CurseForge"
)

type ModpackProject struct {
	ID, Name, Author, Summary, URL, IconURL string
	Source                                  ModpackSource
	distributionRestricted                  bool
}

type ModpackRelease struct {
	ID, Title, Filename, Channel string
	Games, Loaders               []string
	Published                    time.Time
	Size                         int64
	Unavailable                  string
	source                       ModpackSource
	asset                        modpackAsset
	downloadBlocked              bool
}

type ModpackResults struct {
	Projects []ModpackProject
	Total    int
}

func SearchModpacks(ctx context.Context, source ModpackSource, key, query, game string, page int) (ModpackResults, error) {
	if len(query) > 512 || page < 0 || page >= 10000/PackPageSize || (game != "" && !instance.SupportedRelease(game)) {
		return ModpackResults{}, i18n.Error("modpack.search_invalid")
	}
	if source == CurseForgeModpacks {
		return newCurseForge(key).search(ctx, query, game, page)
	}
	if source != ModrinthModpacks {
		return ModpackResults{}, i18n.Error("modpack.source_unknown")
	}
	facets := [][]string{{"project_type:modpack"}}
	if game != "" {
		facets = append(facets, []string{"versions:" + game})
	}
	raw, _ := json.Marshal(facets)
	params := url.Values{"query": {strings.TrimSpace(query)}, "facets": {string(raw)},
		"limit": {strconv.Itoa(PackPageSize)}, "offset": {strconv.Itoa(PackPageSize * page)}}
	if strings.TrimSpace(query) == "" {
		params.Set("index", "downloads")
	}
	var hits PackSearch
	if err := downloadJSON(ctx, &downloads{http: officialHTTP}, modrinthAPI+"search?"+params.Encode(), &hits); err != nil {
		return ModpackResults{}, err
	}
	result := ModpackResults{Total: hits.Total}
	for _, h := range hits.Hits {
		if modrinthID.MatchString(h.ID) && h.Title != "" {
			result.Projects = append(result.Projects, ModpackProject{ID: h.ID, Name: h.Title, Author: h.Author,
				Summary: h.Description, URL: "https://modrinth.com/modpack/" + h.ID, IconURL: h.IconURL, Source: source})
		}
	}
	return result, nil
}

func ListModpackReleases(ctx context.Context, project ModpackProject, key, game string, page int) ([]ModpackRelease, bool, error) {
	if page < 0 || page >= 10000/PackPageSize || (game != "" && !instance.SupportedRelease(game)) {
		return nil, false, i18n.Error("modpack.version_query_invalid")
	}
	if project.Source == CurseForgeModpacks {
		releases, more, err := newCurseForge(key).releases(ctx, project.ID, game, page)
		if project.distributionRestricted {
			for n := range releases {
				releases[n].Unavailable = i18n.Text("modpack.third_party_unavailable")
				releases[n].downloadBlocked = true
			}
		}
		return releases, more, err
	}
	if project.Source != ModrinthModpacks || !modrinthID.MatchString(project.ID) {
		return nil, false, i18n.Error("modpack.project_invalid")
	}
	params := url.Values{"include_changelog": {"false"}}
	if game != "" {
		raw, _ := json.Marshal([]string{game})
		params.Set("game_versions", string(raw))
	}
	var versions []PackVersion
	if err := downloadJSON(ctx, &downloads{http: officialHTTP}, modrinthAPI+"project/"+project.ID+"/version?"+params.Encode(), &versions); err != nil {
		return nil, false, err
	}
	var releases []ModpackRelease
	for _, v := range versions {
		if v.ProjectID != project.ID || !modrinthID.MatchString(v.ID) {
			continue
		}
		find := func(f PackFile) bool { return content.ValidName(f.Filename, ".mrpack") }
		n := slices.IndexFunc(v.Files, func(f PackFile) bool { return f.Primary && find(f) })
		if n < 0 {
			n = slices.IndexFunc(v.Files, find)
		}
		if n < 0 {
			continue
		}
		f := v.Files[n]
		r := ModpackRelease{ID: v.ID, Title: v.Number, Filename: f.Filename, Size: f.Size,
			Channel: v.Type, Published: v.Published, Games: v.GameVersions, Loaders: v.Loaders,
			source: ModrinthModpacks, asset: modpackAsset{URLs: []string{f.URL}, Size: f.Size, Algorithm: "sha512", Digest: f.Hashes.SHA512}}
		r.Unavailable = unsupportedPackTags(r.Games, r.Loaders)
		if r.Unavailable == "" && !r.asset.valid(maxModpackArchive) {
			r.Unavailable = i18n.Text("modpack.download_metadata_missing")
		}
		releases = append(releases, r)
	}
	start := min(page*PackPageSize, len(releases))
	end := min(start+PackPageSize, len(releases))
	return releases[start:end], end < len(releases), nil
}

func unsupportedPackTags(games, loaders []string) string {
	if slices.ContainsFunc(games, instance.IsReleaseID) && !slices.ContainsFunc(games, instance.SupportedRelease) {
		return i18n.Text("version.modern_required")
	}
	if len(loaders) > 0 && !slices.ContainsFunc(loaders, func(s string) bool {
		switch strings.ToLower(s) {
		case "fabric", "forge", "neoforge", "quilt", "minecraft", "vanilla":
			return true
		}
		return false
	}) {
		return i18n.Text("modpack.loader_unsupported")
	}
	return ""
}

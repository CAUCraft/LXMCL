package catalog

import (
	"context"
	"encoding/json"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/distribution"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/modpack"
)

func (c Client) SearchModpacks(ctx context.Context, source ModpackSource, key, query, game string, page int) (ModpackResults, error) {
	if len(query) > 512 || page < 0 || page >= 10000/PackPageSize || (game != "" && !instance.SupportedRelease(game)) {
		return ModpackResults{}, i18n.Error("modpack.search_invalid")
	}
	if source == CurseForgeModpacks {
		return c.curseForge(key).search(ctx, query, game, page)
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
	if err := c.json(ctx, modrinthAPI+"search?"+params.Encode(), "", nil, &hits); err != nil {
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

// ListModpackReleases uses the unmodified SearchModpacks result so that project
// distribution restrictions are carried into each release.
func (c Client) ListModpackReleases(ctx context.Context, project ModpackProject, key, game string, page int) ([]ModpackRelease, bool, error) {
	if page < 0 || page >= 10000/PackPageSize || (game != "" && !instance.SupportedRelease(game)) {
		return nil, false, i18n.Error("modpack.version_query_invalid")
	}
	if project.Source == CurseForgeModpacks {
		releases, more, err := c.curseForge(key).releases(ctx, project.ID, game, page)
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
	if err := c.json(ctx, modrinthAPI+"project/"+project.ID+"/version?"+params.Encode(), "", nil, &versions); err != nil {
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
			source: ModrinthModpacks, asset: distribution.Asset{URLs: []string{f.URL}, Size: f.Size, Algorithm: "sha512", Digest: f.Hashes.SHA512}}
		r.Unavailable = unsupportedPackTags(r.Games, r.Loaders)
		if r.Unavailable == "" && (r.asset.Size <= 0 || !r.asset.Valid(modpack.MaxArchiveSize)) {
			r.Unavailable = i18n.Text("modpack.download_metadata_missing")
		}
		releases = append(releases, r)
	}
	start := min(page*PackPageSize, len(releases))
	end := min(start+PackPageSize, len(releases))
	return releases[start:end], end < len(releases), nil
}

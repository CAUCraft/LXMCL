package launcher

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

type ModpackSource string

const (
	ModrinthModpacks   ModpackSource = "Modrinth"
	CurseForgeModpacks ModpackSource = "CurseForge"
)

type ModpackProject struct {
	ID, Name, Author, Summary, URL string
	Source                         ModpackSource
	distributionRestricted         bool
}

type ModpackRelease struct {
	ID, Title, Filename, Channel string
	Games, Loaders               []string
	Published                    time.Time
	Size                         int64
	Unavailable                  string
	source                       ModpackSource
	asset                        modpackAsset
}

type ModpackResults struct {
	Projects []ModpackProject
	Total    int
}

func SearchModpacks(ctx context.Context, source ModpackSource, key, query, game string, page int) (ModpackResults, error) {
	if len(query) > 512 || page < 0 || page >= 10000/PackPageSize || (game != "" && !modernRelease(game)) {
		return ModpackResults{}, errors.New("整合包搜索条件无效")
	}
	if source == CurseForgeModpacks {
		return newCurseForge(key).search(ctx, query, game, page)
	}
	if source != ModrinthModpacks {
		return ModpackResults{}, errors.New("未知整合包来源")
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
				Summary: h.Description, URL: "https://modrinth.com/modpack/" + h.ID, Source: source})
		}
	}
	return result, nil
}

func ListModpackReleases(ctx context.Context, project ModpackProject, key, game string, page int) ([]ModpackRelease, bool, error) {
	if page < 0 || page >= 10000/PackPageSize || (game != "" && !modernRelease(game)) {
		return nil, false, errors.New("整合包版本查询条件无效")
	}
	if project.Source == CurseForgeModpacks {
		releases, more, err := newCurseForge(key).releases(ctx, project.ID, game, page)
		if project.distributionRestricted {
			for n := range releases {
				releases[n].Unavailable = "此整合包未开放给第三方启动器下载"
			}
		}
		return releases, more, err
	}
	if project.Source != ModrinthModpacks || !modrinthID.MatchString(project.ID) {
		return nil, false, errors.New("整合包项目无效")
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
		find := func(f PackFile) bool { return contentName(f.Filename, ".mrpack") }
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
			r.Unavailable = "缺少有效的整合包下载地址或校验值"
		}
		releases = append(releases, r)
	}
	start := min(page*PackPageSize, len(releases))
	end := min(start+PackPageSize, len(releases))
	return releases[start:end], end < len(releases), nil
}

func unsupportedPackTags(games, loaders []string) string {
	if slices.ContainsFunc(games, releaseID.MatchString) && !slices.ContainsFunc(games, modernRelease) {
		return "需要 Minecraft 1.20.1 或之后的正式版"
	}
	if len(loaders) > 0 && !slices.ContainsFunc(loaders, func(s string) bool {
		switch strings.ToLower(s) {
		case "fabric", "forge", "neoforge", "minecraft", "vanilla":
			return true
		}
		return false
	}) {
		return "需要原版、Fabric、Forge 或 NeoForge"
	}
	return ""
}

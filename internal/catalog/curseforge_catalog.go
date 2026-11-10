package catalog

import (
	"context"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/modpack"
)

var versionTag = regexp.MustCompile(`^[0-9][0-9A-Za-z._-]{0,63}$`)

func curseParams(game string, page int) url.Values {
	p := url.Values{"index": {strconv.Itoa(page * PackPageSize)}, "pageSize": {strconv.Itoa(PackPageSize)}}
	if game != "" {
		p.Set("gameVersion", game)
	}
	return p
}

func (cf curseForge) search(ctx context.Context, query, game string, page int) (ModpackResults, error) {
	p := curseParams(game, page)
	p.Set("gameId", strconv.Itoa(curseGameID))
	p.Set("classId", strconv.Itoa(curseModpacksClass))
	if strings.TrimSpace(query) != "" {
		p.Set("searchFilter", strings.TrimSpace(query))
	} else {
		p.Set("sortField", "6")
		p.Set("sortOrder", "desc")
	}
	var data cursePage[curseProject]
	if err := cf.call(ctx, "mods/search?"+p.Encode(), nil, &data); err != nil {
		return ModpackResults{}, err
	}
	result := ModpackResults{Total: data.Pagination.TotalCount}
	for _, m := range data.Data {
		if m.ID <= 0 || m.GameID != curseGameID || m.ClassID != curseModpacksClass || !m.IsAvailable {
			continue
		}
		p := ModpackProject{ID: strconv.Itoa(m.ID), Name: m.Name, Summary: m.Summary, Source: CurseForgeModpacks, IconURL: m.Logo.ThumbnailURL}
		if p.IconURL == "" {
			p.IconURL = m.Logo.URL
		}
		p.distributionRestricted = m.AllowModDistribution != nil && !*m.AllowModDistribution
		if len(m.Authors) > 0 {
			p.Author = m.Authors[0].Name
		}
		if m.Slug != "" {
			p.URL = "https://www.curseforge.com/minecraft/modpacks/" + url.PathEscape(m.Slug)
		}
		result.Projects = append(result.Projects, p)
	}
	return result, nil
}

func (cf curseForge) releases(ctx context.Context, id, game string, page int) ([]ModpackRelease, bool, error) {
	projectID, err := strconv.Atoi(id)
	if err != nil || projectID <= 0 {
		return nil, false, i18n.Error("curseforge.project_id_invalid")
	}
	var data cursePage[curseFile]
	if err := cf.call(ctx, "mods/"+strconv.Itoa(projectID)+"/files?"+curseParams(game, page).Encode(), nil, &data); err != nil {
		return nil, false, err
	}
	var releases []ModpackRelease
	for _, f := range data.Data {
		if f.ModID != projectID || f.GameID != curseGameID || f.ID <= 0 || f.IsServerPack || !content.ValidName(f.FileName, ".zip") {
			continue
		}
		r := ModpackRelease{ID: strconv.Itoa(f.ID), Title: f.DisplayName, Filename: f.FileName,
			Published: f.FileDate, Size: f.FileLength, source: CurseForgeModpacks, asset: f.asset(),
			downloadBlocked: !f.IsAvailable || f.IsEarlyAccessContent || f.DownloadURL == ""}
		r.Channel = map[int]string{1: "release", 2: "beta", 3: "alpha"}[f.ReleaseType]
		for _, tag := range f.GameVersions {
			if versionTag.MatchString(tag) {
				r.Games = append(r.Games, tag)
			}
			if slices.Contains([]string{"fabric", "forge", "neoforge", "quilt", "vanilla"}, strings.ToLower(tag)) {
				r.Loaders = append(r.Loaders, tag)
			}
		}
		r.Unavailable = unsupportedPackTags(r.Games, r.Loaders)
		if r.Unavailable == "" {
			switch {
			case r.downloadBlocked:
				r.Unavailable = i18n.Text("curseforge.file_unavailable")
			case !r.asset.Valid(modpack.MaxArchiveSize):
				r.Unavailable = i18n.Text("modpack.download_metadata_missing")
			}
		}
		releases = append(releases, r)
	}
	return releases, data.Pagination.ResultCount > 0 && data.Pagination.Index+data.Pagination.ResultCount < data.Pagination.TotalCount, nil
}

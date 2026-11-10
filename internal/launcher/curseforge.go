package launcher

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

const (
	curseGameID        = 432
	curseModpacksClass = 4471
)

var curseHTTP = &http.Client{
	Timeout:       30 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

type curseForge struct {
	key  string
	http *http.Client
}

func newCurseForge(key string) curseForge { return curseForge{strings.TrimSpace(key), curseHTTP} }

type curseProject struct {
	ID, GameID, ClassID  int
	Name, Summary, Slug  string
	IsAvailable          bool
	AllowModDistribution *bool
	Logo                 struct{ ThumbnailURL, URL string }
	Authors              []struct{ Name string }
}

type curseFile struct {
	ID, ModID, GameID, ReleaseType                  int
	DisplayName, FileName, DownloadURL              string
	FileLength                                      int64
	IsAvailable, IsServerPack, IsEarlyAccessContent bool
	FileDate                                        time.Time
	GameVersions                                    []string
	Hashes                                          []struct {
		Algo  int
		Value string
	}
}

type cursePage[T any] struct {
	Data       []T
	Pagination struct{ Index, ResultCount, TotalCount int }
}

func (cf curseForge) call(ctx context.Context, endpoint string, payload any, dst any) error {
	if cf.key == "" {
		return i18n.Error("curseforge.api_key_required")
	}
	var body []byte
	method := http.MethodGet
	if payload != nil {
		body, _ = json.Marshal(payload)
		method = http.MethodPost
	}
	return retry(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, method, "https://api.curseforge.com/v1/"+endpoint, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("x-api-key", cf.key)
		req.Header.Set("Accept", "application/json")
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := cf.http.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return i18n.Error("curseforge.access_denied")
		case http.StatusOK:
			b, err := io.ReadAll(io.LimitReader(resp.Body, maxMetadata+1))
			if err != nil {
				return err
			}
			if len(b) > maxMetadata {
				return i18n.Error("curseforge.response_too_large")
			}
			if err := json.Unmarshal(b, dst); err != nil {
				return i18n.Error("curseforge.read_failed", err)
			}
			return nil
		default:
			return &responseError{status: resp.StatusCode, src: "CurseForge", delay: retryAfter(resp.Header.Get("Retry-After"), time.Now())}
		}
	})
}

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

func (f curseFile) asset() modpackAsset {
	a := modpackAsset{URLs: []string{f.DownloadURL}, Size: f.FileLength}
	u, err := url.Parse(f.DownloadURL)
	if err != nil || !curseCDN(u) || f.FileLength <= 0 {
		return modpackAsset{}
	}
	for _, algo := range []int{1, 2} {
		for _, h := range f.Hashes {
			if h.Algo != algo {
				continue
			}
			a.Digest, a.Algorithm = h.Value, "sha1"
			if algo == 2 {
				a.Algorithm = "md5"
			}
			if a.valid(maxModpackFile) {
				return a
			}
		}
	}
	return modpackAsset{}
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
			if versionID.MatchString(tag) {
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
			case !r.asset.valid(maxModpackArchive):
				r.Unavailable = i18n.Text("modpack.download_metadata_missing")
			}
		}
		releases = append(releases, r)
	}
	return releases, data.Pagination.ResultCount > 0 && data.Pagination.Index+data.Pagination.ResultCount < data.Pagination.TotalCount, nil
}

type curseReference struct {
	ProjectID, FileID int
	Required          *bool
}

func (cf curseForge) resolve(ctx context.Context, refs []curseReference) ([]modpackDownload, error) {
	var projectIDs, fileIDs []int
	projects, files := make(map[int]curseProject), make(map[int]curseFile)
	for _, ref := range refs {
		if ref.ProjectID <= 0 || ref.FileID <= 0 {
			return nil, i18n.Error("curseforge.manifest_reference_invalid")
		}
		projectIDs = append(projectIDs, ref.ProjectID)
		fileIDs = append(fileIDs, ref.FileID)
	}
	for ids := range slices.Chunk(uniquePackIDs(projectIDs), 50) {
		var data cursePage[curseProject]
		if err := cf.call(ctx, "mods", map[string][]int{"modIds": ids}, &data); err != nil {
			return nil, err
		}
		for _, p := range data.Data {
			projects[p.ID] = p
		}
	}
	for ids := range slices.Chunk(uniquePackIDs(fileIDs), 50) {
		var data cursePage[curseFile]
		if err := cf.call(ctx, "mods/files", map[string][]int{"fileIds": ids}, &data); err != nil {
			return nil, err
		}
		for _, f := range data.Data {
			files[f.ID] = f
		}
	}
	var result []modpackDownload
	for _, ref := range refs {
		p, f := projects[ref.ProjectID], files[ref.FileID]
		name := p.Name
		if name == "" {
			name = i18n.Text("curseforge.project_label", ref.ProjectID)
		}
		if p.GameID != curseGameID || f.GameID != curseGameID || f.ModID != ref.ProjectID || !p.IsAvailable || !f.IsAvailable ||
			f.IsEarlyAccessContent || f.IsServerPack || f.DownloadURL == "" || (p.AllowModDistribution != nil && !*p.AllowModDistribution) {
			return nil, i18n.Error("curseforge.dependency_unavailable", name, ref.FileID)
		}
		dir := map[int]string{6: "mods", 12: "resourcepacks", 6552: "shaderpacks"}[p.ClassID]
		a := f.asset()
		if dir == "" || !storage.ValidPortablePath(f.FileName) || strings.Contains(f.FileName, "/") || !a.valid(maxModpackFile) {
			return nil, i18n.Error("curseforge.dependency_invalid", name, ref.FileID)
		}
		result = append(result, modpackDownload{path.Join(dir, f.FileName), a})
	}
	return result, nil
}

func uniquePackIDs(ids []int) []int {
	slices.Sort(ids)
	return slices.Compact(ids)
}

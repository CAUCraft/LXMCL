package minecraft

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

const (
	releaseCatalogCacheFile = "official-releases.json"
	maxCachedReleases       = 4096
)

type releaseCatalogCache struct {
	Releases []string `json:"releases"`
}

// CachedReleases reads the most recently saved catalog for browsing without a
// network request. No catalog is distinct from an unreadable or invalid cache.
func (g Runtime) CachedReleases(ctx context.Context) ([]Version, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(g.Cache) {
		return nil, i18n.Error("distribution.cache_directory_required")
	}
	root, err := os.OpenRoot(g.Cache)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	entry, err := root.Lstat(releaseCatalogCacheFile)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !entry.Mode().IsRegular() || entry.Size() > maxMetadata {
		return nil, i18n.Error("version.catalog_cache_invalid")
	}
	file, err := root.Open(releaseCatalogCacheFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || !os.SameFile(entry, info) || info.Size() > maxMetadata {
		return nil, i18n.Error("version.catalog_cache_invalid")
	}
	data, err := io.ReadAll(io.LimitReader(transfer.Reader(ctx, file), maxMetadata+1))
	if err != nil {
		return nil, err
	}
	var cache releaseCatalogCache
	if len(data) > maxMetadata || json.Unmarshal(data, &cache) != nil {
		return nil, i18n.Error("version.catalog_cache_invalid")
	}
	versions, ok := browsingReleases(cache.Releases)
	if !ok {
		return nil, i18n.Error("version.catalog_cache_invalid")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return versions, nil
}

// RefreshReleases fetches and validates the official catalog. An error with a
// nonempty result means the fresh list is usable, but saving it failed.
func (g Runtime) RefreshReleases(ctx context.Context) ([]Version, error) {
	d := g.Client.downloads()
	client := *d.http
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !mojangMetadataURL(req.URL) || req.URL.Path != "/mc/game/version_manifest_v2.json" {
			return i18n.Error("download.redirect_denied")
		}
		return nil
	}
	d.http = &client
	releases, err := d.releases(ctx)
	if err != nil {
		return nil, err
	}
	if len(releases) > maxCachedReleases {
		return nil, i18n.Error("version.catalog_release_invalid")
	}
	ids := make([]string, 0, len(releases))
	for _, release := range releases {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		metadataURL, err := url.Parse(release.URL)
		if err != nil || !mojangMetadataURL(metadataURL) {
			return nil, i18n.Error("version.catalog_release_invalid")
		}
		if _, err := (artifact{SHA: release.SHA, Size: unknownSize}).Checksum(); err != nil {
			return nil, i18n.Error("version.catalog_release_invalid")
		}
		ids = append(ids, release.ID)
	}
	versions, ok := browsingReleases(ids)
	if !ok {
		return nil, i18n.Error("version.catalog_release_invalid")
	}
	if err := g.saveReleaseCatalog(ctx, releaseCatalogCache{Releases: ids}); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return versions, i18n.Error("version.catalog_cache_save_failed", err)
	}
	return versions, nil
}

func mojangMetadataURL(u *url.URL) bool {
	if !officialURL(u) || u.Host != u.Hostname() || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" ||
		u.EscapedPath() != u.Path || path.Clean(u.Path) != u.Path || !strings.HasSuffix(u.Path, ".json") {
		return false
	}
	switch u.Hostname() {
	case "piston-meta.mojang.com", "piston-data.mojang.com", "launchermeta.mojang.com", "launcher.mojang.com":
		return true
	default:
		return false
	}
}

func browsingReleases(ids []string) ([]Version, bool) {
	if len(ids) == 0 || len(ids) > maxCachedReleases {
		return nil, false
	}
	versions := make([]Version, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if !instance.SupportedRelease(id) || seen[id] {
			return nil, false
		}
		seen[id] = true
		versions = append(versions, Version{ID: id, Type: "release"})
	}
	return versions, true
}

func (g Runtime) saveReleaseCatalog(ctx context.Context, cache releaseCatalogCache) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !filepath.IsAbs(g.Cache) {
		return i18n.Error("distribution.cache_directory_required")
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(g.Cache, 0755); err != nil {
		return err
	}
	root, err := os.OpenRoot(g.Cache)
	if err != nil {
		return err
	}
	defer root.Close()
	return storage.ReplaceFile(ctx, root, releaseCatalogCacheFile, 0600, func(file *os.File) error {
		_, err := file.Write(data)
		return err
	})
}

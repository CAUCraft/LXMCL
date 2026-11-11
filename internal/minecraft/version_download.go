package minecraft

import (
	"context"
	"path/filepath"
	"slices"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

func (g Runtime) downloadVersion(ctx context.Context, id string, d *downloads) (metadata, Version, error) {
	all, err := d.releases(ctx)
	if err != nil {
		return metadata{}, Version{}, err
	}
	index := slices.IndexFunc(all, func(v Version) bool { return v.ID == id })
	if index == -1 {
		return metadata{}, Version{}, i18n.Error("version.release_not_found", id)
	}
	v := all[index]
	if err := d.file(ctx, filepath.Join(g.versionDir(id), "version.json"), artifact{URL: v.URL, SHA: v.SHA, Size: unknownSize}); err != nil {
		return metadata{}, Version{}, err
	}
	m, err := g.readVersion(id, v.SHA)
	return m, v, err
}

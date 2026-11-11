package minecraft

import (
	"context"
	"slices"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

const manifestURL = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"

type Version struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	URL  string `json:"url"`
	SHA  string `json:"sha1"`
}

func (c Client) Catalog(ctx context.Context) ([]Version, error) {
	return c.downloads().catalog(ctx)
}

func (d *downloads) catalog(ctx context.Context) ([]Version, error) {
	var data struct{ Versions []Version }
	if err := downloadJSON(ctx, d, manifestURL, &data); err != nil {
		return nil, err
	}
	if len(data.Versions) == 0 {
		return nil, i18n.Error("version.catalog_empty")
	}
	return data.Versions, nil
}

func (c Client) Releases(ctx context.Context) ([]Version, error) {
	return c.downloads().releases(ctx)
}

func (d *downloads) releases(ctx context.Context) ([]Version, error) {
	all, err := d.catalog(ctx)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(all, func(v Version) bool {
		return v.Type != "release" || !instance.SupportedRelease(v.ID)
	}), nil
}

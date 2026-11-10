package launcher

import (
	"context"
	"encoding/json"
	"io"
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

func Catalog(ctx context.Context) ([]Version, error) {
	return (&downloads{http: officialHTTP}).catalog(ctx)
}

func (d *downloads) catalog(ctx context.Context) ([]Version, error) {
	var data struct{ Versions []Version }
	if err := retry(ctx, func() error {
		resp, err := d.get(ctx, manifestURL)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		return json.NewDecoder(io.LimitReader(resp.Body, maxMetadata)).Decode(&data)
	}); err != nil {
		return nil, err
	}
	if len(data.Versions) == 0 {
		return nil, i18n.Error("version.catalog_empty")
	}
	return data.Versions, nil
}

func Releases(ctx context.Context) ([]Version, error) {
	return (&downloads{http: officialHTTP}).releases(ctx)
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

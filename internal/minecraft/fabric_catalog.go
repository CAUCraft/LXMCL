package minecraft

import (
	"context"
	"net/url"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

func (c Client) FabricVersions(ctx context.Context, gameVersion string) ([]string, error) {
	if !instance.SupportedRelease(gameVersion) {
		return nil, i18n.Error("version.select_supported")
	}
	var entries []struct {
		Loader struct {
			Version string
			Stable  bool
		}
	}
	if err := downloadJSON(ctx, c.downloads(), fabricMeta+url.PathEscape(gameVersion), &entries); err != nil {
		return nil, err
	}
	var stable []string
	for _, entry := range entries {
		if entry.Loader.Stable && instance.ValidKnotVersion(entry.Loader.Version) {
			stable = append(stable, entry.Loader.Version)
		}
	}
	return stable, nil
}

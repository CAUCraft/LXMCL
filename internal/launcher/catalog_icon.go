package launcher

import (
	"context"
	"image"

	"github.com/CAUCraft/LXMCL/internal/catalog"
)

func LoadCatalogIcon(ctx context.Context, raw string) (image.Image, error) {
	return (catalog.Client{}).LoadIcon(ctx, raw)
}

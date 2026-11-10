package launcher

import (
	"context"

	"github.com/CAUCraft/LXMCL/internal/minecraft"
)

type Version = minecraft.Version
type LoaderRelease = minecraft.LoaderRelease

func Catalog(ctx context.Context) ([]Version, error)  { return (minecraft.Client{}).Catalog(ctx) }
func Releases(ctx context.Context) ([]Version, error) { return (minecraft.Client{}).Releases(ctx) }
func FabricVersions(ctx context.Context, gameVersion string) ([]string, error) {
	return (minecraft.Client{}).FabricVersions(ctx, gameVersion)
}
func LoaderVersions(ctx context.Context, game, kind string) ([]LoaderRelease, error) {
	return (minecraft.Client{}).LoaderVersions(ctx, game, kind)
}

package launcher

import (
	"context"

	"github.com/CAUCraft/LXMCL/internal/catalog"
)

type ModpackSource = catalog.ModpackSource

const (
	ModrinthModpacks   = catalog.ModrinthModpacks
	CurseForgeModpacks = catalog.CurseForgeModpacks
)

type ModpackProject = catalog.ModpackProject
type ModpackRelease = catalog.ModpackRelease
type ModpackResults = catalog.ModpackResults

func SearchModpacks(ctx context.Context, source ModpackSource, key, query, game string, page int) (ModpackResults, error) {
	return (catalog.Client{}).SearchModpacks(ctx, source, key, query, game, page)
}

func ListModpackReleases(ctx context.Context, project ModpackProject, key, game string, page int) ([]ModpackRelease, bool, error) {
	return (catalog.Client{}).ListModpackReleases(ctx, project, key, game, page)
}

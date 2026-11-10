package launcher

import (
	"context"

	"github.com/CAUCraft/LXMCL/internal/catalog"
)

type PackProject = catalog.PackProject
type ResourceFilter = catalog.ResourceFilter
type PackVersion = catalog.PackVersion
type ModDependency = catalog.ModDependency
type PackFile = catalog.PackFile
type PackSearch = catalog.PackSearch

const PackPageSize = catalog.PackPageSize

func SearchModrinth(ctx context.Context, i Instance, kind ContentKind, query, order string, page int) (PackSearch, error) {
	return (catalog.Client{}).SearchModrinth(ctx, i, kind, query, order, page)
}

func SearchResourceCatalog(ctx context.Context, f ResourceFilter, kind ContentKind, query, order string, page int) (PackSearch, error) {
	return (catalog.Client{}).SearchResourceCatalog(ctx, f, kind, query, order, page)
}

func ModrinthVersions(ctx context.Context, project string, i Instance, kind ContentKind) ([]PackVersion, error) {
	return (catalog.Client{}).ModrinthVersions(ctx, project, i, kind)
}

func ResourceCatalogVersions(ctx context.Context, project string, f ResourceFilter, kind ContentKind) ([]PackVersion, error) {
	return (catalog.Client{}).ResourceCatalogVersions(ctx, project, f, kind)
}

func ModrinthVersion(ctx context.Context, project, id string) (PackVersion, error) {
	return (catalog.Client{}).ModrinthVersion(ctx, project, id)
}

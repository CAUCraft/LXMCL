package launcher

import (
	"context"

	"github.com/CAUCraft/LXMCL/internal/catalog"
)

type DependencyCatalog = catalog.DependencyCatalog

func ResourceDependencyProjects(ctx context.Context, deps []ModDependency) (map[string]PackProject, error) {
	return (catalog.Client{}).ResourceDependencyProjects(ctx, deps)
}

func OpenResourceDependency(ctx context.Context, dep ModDependency, target Instance, filter ResourceFilter) (DependencyCatalog, error) {
	return (catalog.Client{}).OpenResourceDependency(ctx, dep, target, filter)
}

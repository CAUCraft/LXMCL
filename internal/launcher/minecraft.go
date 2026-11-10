package launcher

import (
	"context"

	"github.com/CAUCraft/LXMCL/internal/minecraft"
	"github.com/CAUCraft/LXMCL/internal/tasks"
)

// Game coordinates backend services for a launcher cache.
type Game struct{ Cache string }
type Progress = tasks.Progress

func (g Game) runtime() minecraft.Runtime { return minecraft.Runtime{Cache: g.Cache} }
func (g Game) Installed(i Instance) bool  { return g.runtime().Installed(i) }
func (g Game) Install(ctx context.Context, i Instance, report func(Progress)) error {
	return g.runtime().Install(ctx, i, report)
}
func (g Game) InstallWithJava(ctx context.Context, i Instance, javaPath string, report func(Progress)) error {
	return g.runtime().InstallWithJava(ctx, i, javaPath, report)
}

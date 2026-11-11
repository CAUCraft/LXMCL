package launcher

import (
	"context"
	"net/http"

	"github.com/CAUCraft/LXMCL/internal/distribution"
	"github.com/CAUCraft/LXMCL/internal/minecraft"
	"github.com/CAUCraft/LXMCL/internal/tasks"
)

// Game coordinates installations and process startup for a launcher cache.
// Transport handles installation and startup preparation; nil uses the standard
// transport. Execute delegates account authorization to the supplied Store.
// Configure it before use. It must honor contexts and support concurrent requests;
// each domain client retains its timeout, credential and redirect policies.
type Game struct {
	Cache     string
	Transport http.RoundTripper
}
type Progress = tasks.Progress

func (g Game) runtime() minecraft.Runtime {
	return minecraft.Runtime{Cache: g.Cache, Client: minecraft.Client{Transport: g.Transport}}
}
func (g Game) downloads() distribution.Cache {
	return distribution.Cache{Root: g.Cache, Client: &http.Client{Transport: g.Transport}}
}
func (g Game) Installed(i Instance) bool { return g.runtime().Installed(i) }
func (g Game) Install(ctx context.Context, i Instance, report func(Progress)) error {
	return g.runtime().Install(ctx, i, report)
}
func (g Game) InstallWithJava(ctx context.Context, i Instance, javaPath string, report func(Progress)) error {
	return g.runtime().InstallWithJava(ctx, i, javaPath, report)
}

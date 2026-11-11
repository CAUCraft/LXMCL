package launcher

import (
	"context"

	"github.com/CAUCraft/LXMCL/internal/javaruntime"
)

type Java = javaruntime.Runtime

func ProbeJava(ctx context.Context, path string) (Java, error) { return javaruntime.Probe(ctx, path) }
func FindJava(ctx context.Context, path string, minimum int) (Java, error) {
	return javaruntime.Find(ctx, path, minimum)
}
func (g Game) CheckJava(ctx context.Context, i Instance, globalPath string) (Java, int, error) {
	return g.runtime().CheckJava(ctx, i, globalPath)
}

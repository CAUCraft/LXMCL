package minecraft

import (
	"context"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/javaruntime"
)

type Java = javaruntime.Runtime

// CheckJava checks local Java against the game's requirements without installing
// a runtime or game. A new instance only downloads official version metadata.
func (g Runtime) CheckJava(ctx context.Context, i instance.Instance, globalPath string) (Java, int, error) {
	if err := i.Validate(); err != nil {
		return Java{}, 0, err
	}
	if err := i.Launch.Validate(); err != nil {
		return Java{}, 0, err
	}
	receipt, err := g.readReceipt(i.Version)
	var m metadata
	if err == nil {
		m, err = g.readVersion(i.Version, receipt.VersionSHA)
	}
	if err != nil {
		m, _, err = g.downloadVersion(ctx, i.Version, g.Client.downloads())
		if err != nil {
			return Java{}, 0, i18n.Error("java.requirement_read_failed", err)
		}
	}
	minimum := m.JavaVersion.MajorVersion
	manager := javaruntime.Manager{Root: filepath.Join(g.Cache, "java"), Client: g.Client.downloads().http}
	j, err := manager.Local(ctx, javaruntime.Requirement{Component: m.JavaVersion.Component, Major: minimum}, i.Launch.JavaExecutable(globalPath))
	return j, minimum, err
}

func (g Runtime) resolveJava(ctx context.Context, preferred string, m metadata, report func(Progress)) (Java, error) {
	manager := javaruntime.Manager{Root: filepath.Join(g.Cache, "java"), Client: g.Client.downloads().http}
	return manager.Resolve(ctx, javaruntime.Requirement{Component: m.JavaVersion.Component, Major: m.JavaVersion.MajorVersion}, preferred, func(done, total int) {
		if report != nil {
			report(Progress{Stage: i18n.Text("progress.install_java"), Done: done, Total: total})
		}
	})
}

package minecraft

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

type launchProfile struct {
	metadata            metadata
	classpath           []string
	libraryDir, version string
}

func (g Runtime) prepareProfile(ctx context.Context, i instance.Instance) (launchProfile, error) {
	m, cp, err := g.verifyInstalled(ctx, i)
	libraryDir, launchVersion := filepath.Join(g.Cache, "libraries"), m.ID
	if err == nil && i.UsesKnot() {
		var profile knotRuntime
		var libs []string
		profile, libs, err = g.verifyKnot(ctx, i)
		if err == nil {
			cp = append(libs, cp...)
			m.MainClass = profile.MainClass
			m.Arguments.JVM = append(m.Arguments.JVM, profile.Arguments.JVM...)
			m.Arguments.Game = append(m.Arguments.Game, profile.Arguments.Game...)
		}
	}
	if err == nil && i.UsesInstaller() {
		var profile forgeProfile
		profile, err = g.readForge(i)
		if err == nil {
			var libs []string
			libs, err = g.verifyForge(ctx, i, profile)
			if err == nil {
				cp = g.mergeForgeClasspath(m, profile, cp, libs)
				m.MainClass = profile.MainClass
				m.Arguments.JVM = append(m.Arguments.JVM, profile.Arguments.JVM...)
				m.Arguments.Game = append(m.Arguments.Game, profile.Arguments.Game...)
				launchVersion = profile.ID
				libraryDir = filepath.Join(g.forgeDir(i), "libraries")
			}
		}
	}
	if err != nil {
		return launchProfile{}, err
	}
	return launchProfile{m, cp, libraryDir, launchVersion}, nil
}

func validateClasspath(paths []string) error {
	for _, path := range paths {
		if strings.ContainsRune(path, os.PathListSeparator) {
			return i18n.Error("launch.cache_path_separator", os.PathListSeparator)
		}
	}
	return nil
}

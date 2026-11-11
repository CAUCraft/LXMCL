package javaruntime

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

func (m Manager) managed(ctx context.Context, req Requirement) (Runtime, error) {
	root := filepath.Join(m.Root, "windows-x64", req.Component)
	dirs, err := os.ReadDir(root)
	if err != nil {
		return Runtime{}, err
	}
	for _, dir := range dirs {
		suffix, ok := strings.CutPrefix(dir.Name(), "runtime-")
		if !dir.IsDir() || !ok {
			continue
		}
		// Existing cache names retain the trusted manifest digest from installation.
		sha, _, _ := strings.Cut(suffix, "-")
		expected := transfer.Artifact{SHA: sha, Size: transfer.UnknownSize}
		if found, err := m.verify(ctx, filepath.Join(root, dir.Name()), expected, req); err == nil {
			return found, nil
		}
		if ctx.Err() != nil {
			return Runtime{}, ctx.Err()
		}
	}
	return Runtime{}, i18n.Error("java.compatible_not_found", req.Major)
}

// verify requires an explicit manifest description for both staged and cached
// runtimes. The directory name never decides whether verification is needed.
func (m Manager) verify(ctx context.Context, dir string, expected transfer.Artifact, req Requirement) (Runtime, error) {
	manifest, err := loadManifest(ctx, filepath.Join(dir, "manifest.json"), expected)
	if err != nil {
		return Runtime{}, err
	}
	if err = manifest.verify(ctx, dir); err != nil {
		return Runtime{}, err
	}
	j, err := m.inspect(ctx, dir)
	if err != nil {
		return Runtime{}, err
	}
	if j.Major < req.Major {
		return Runtime{}, i18n.Error("java.version_too_old", j.Major, req.Major)
	}
	return j, nil
}

func (m Manager) inspect(ctx context.Context, dir string) (Runtime, error) {
	probe := m.probe
	if probe == nil {
		probe = Probe
	}
	return probe(ctx, filepath.Join(dir, "bin", "java.exe"))
}

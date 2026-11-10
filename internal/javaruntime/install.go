package javaruntime

import (
	"context"
	"os"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

func (m Manager) install(ctx context.Context, root string, req Requirement, report Progress) (Runtime, error) {
	catalog, err := m.catalog(ctx)
	if err != nil {
		return Runtime{}, err
	}
	releases := catalog["windows-x64"][req.Component]
	if len(releases) == 0 {
		return Runtime{}, i18n.Error("java.runtime_unavailable", req.Component)
	}
	// The catalog orders the currently published runtime first.
	asset := releases[0].Manifest
	if asset.Size <= 0 || asset.Size > metadataLimit {
		return Runtime{}, i18n.Error("java.runtime_manifest_invalid")
	}
	stage, err := os.MkdirTemp(root, ".install-")
	if err != nil {
		return Runtime{}, err
	}
	defer os.RemoveAll(stage)
	if err = asset.Ensure(ctx, filepath.Join(stage, "manifest.json"), metadataLimit, m.fetch); err != nil {
		return Runtime{}, err
	}
	files, err := readManifest(filepath.Join(stage, "manifest.json"))
	if err != nil {
		return Runtime{}, err
	}
	if err = files.validate(); err != nil {
		return Runtime{}, err
	}
	total := 0
	for _, f := range files.Files {
		if f.Type == "file" {
			total++
		}
	}
	if report != nil {
		report(0, total)
	}
	done := 0
	for name, f := range files.Files {
		path := filepath.Join(stage, filepath.FromSlash(name))
		if f.Type == "directory" {
			err = os.MkdirAll(path, 0755)
		} else {
			err = f.Downloads.Raw.Ensure(ctx, path, metadataLimit, m.fetch)
			if err == nil {
				done++
				if report != nil {
					report(done, total)
				}
			}
		}
		if err != nil {
			return Runtime{}, err
		}
	}
	j, err := m.verify(ctx, stage, req)
	if err != nil {
		return Runtime{}, err
	}
	if err = ctx.Err(); err != nil {
		return Runtime{}, err
	}
	// A unique publication directory preserves previous installs and avoids overwrite.
	destination := filepath.Join(root, "runtime-"+asset.SHA+"-"+filepath.Base(stage)[len(".install-"):])
	if err = os.Rename(stage, destination); err != nil {
		return Runtime{}, err
	}
	return Runtime{Path: filepath.Join(destination, "bin", "java.exe"), Major: j.Major}, nil
}

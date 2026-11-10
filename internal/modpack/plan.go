package modpack

import (
	"archive/zip"
	"context"
	"path"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/distribution"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

// Reference identifies one required CurseForge file in a manifest.
type Reference struct {
	ProjectID, FileID int
}

// Download describes a verified asset and its portable path within the instance.
type Download struct {
	Path  string
	Asset distribution.Asset
}

// Resolver returns exactly one download per required reference, in input order.
// The trusted adapter verifies project/file identity, availability and distribution
// permissions; Install checks result count, file descriptors and safe target paths.
type Resolver interface {
	Resolve(context.Context, []Reference) ([]Download, error)
}

type override struct {
	path string
	file *zip.File
}

type plan struct {
	info      Info
	downloads []Download
	curse     []Reference
	overrides []override
}

func (p plan) validate() error {
	if len(p.downloads) > maxEntries {
		return i18n.Error("modpack.too_many_dependencies")
	}
	type entry struct {
		path string
		file bool
	}
	paths := make(map[string]entry)
	var total int64
	add := func(name string, overwrite bool, size int64) error {
		total += size
		if total > maxBytes || !storage.ValidPortablePath(name) {
			return i18n.Error("modpack.plan_size_or_path_invalid", name)
		}
		key := strings.ToLower(name)
		if old, ok := paths[key]; ok && (!overwrite || old != (entry{name, true})) {
			return i18n.Error("modpack.plan_path_conflict", name)
		}
		paths[key] = entry{name, true}
		for dir := path.Dir(name); dir != "."; dir = path.Dir(dir) {
			key := strings.ToLower(dir)
			if old, ok := paths[key]; ok && old != (entry{dir, false}) {
				return i18n.Error("modpack.plan_directory_conflict", dir)
			}
			paths[key] = entry{dir, false}
		}
		return nil
	}
	for _, d := range p.downloads {
		if !d.Asset.Valid(distribution.MaxFileSize) {
			return i18n.Error("modpack.dependency_download_invalid", d.Path)
		}
		if err := add(d.Path, false, d.Asset.Size); err != nil {
			return err
		}
	}
	for _, o := range p.overrides {
		if err := add(o.path, true, int64(o.file.UncompressedSize64)); err != nil {
			return err
		}
	}
	return nil
}

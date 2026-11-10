package modpack

import (
	"archive/zip"
	"io"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/distribution"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

func readManifest(f *zip.File, dst any) error {
	if err := storage.ReadArchiveJSON(f, maxManifestSize, dst); err != nil {
		return i18n.Error("modpack.manifest_read_failed", err)
	}
	return nil
}

func readPlan(r io.ReaderAt, size int64, source Source) (plan, error) {
	p := plan{info: Info{Source: source}}
	z, err := zip.NewReader(r, size)
	if err != nil {
		return p, i18n.Error("modpack.archive_open_failed", err)
	}
	if len(z.File) > maxEntries {
		return p, i18n.Error("modpack.archive_too_many_files")
	}
	files := make(map[string]*zip.File, len(z.File))
	seen := make(map[string]bool, len(z.File))
	var bytes uint64
	for _, f := range z.File {
		dir := f.Mode().IsDir() && strings.HasSuffix(f.Name, "/")
		name := strings.TrimSuffix(f.Name, "/")
		key := strings.ToLower(name)
		if !storage.ValidPortablePath(name) || seen[key] || (!dir && (!f.Mode().IsRegular() || f.Name != name)) {
			return p, i18n.Error("modpack.archive_path_invalid", f.Name)
		}
		seen[key] = true
		if f.UncompressedSize64 > distribution.MaxFileSize || bytes+f.UncompressedSize64 > maxBytes {
			return p, i18n.Error("modpack.archive_expanded_too_large")
		}
		bytes += f.UncompressedSize64
		if !dir {
			files[name] = f
		}
	}
	var layers []string
	switch source {
	case Modrinth:
		layers, err = readModrinth(&p, files)
	case CurseForge:
		layers, err = readCurseForge(&p, files)
	default:
		return p, i18n.Error("modpack.source_unknown")
	}
	if err != nil {
		return p, err
	}
	for _, layer := range layers {
		for _, f := range z.File {
			name, ok := strings.CutPrefix(f.Name, layer+"/")
			if ok && !f.Mode().IsDir() {
				p.overrides = append(p.overrides, override{name, f})
			}
		}
	}
	p.info.Downloads, p.info.Overrides = len(p.downloads)+len(p.curse), len(p.overrides)
	return p, p.validate()
}

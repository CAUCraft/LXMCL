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
	files, err := storage.IndexPackEntries(z.File, distribution.MaxFileSize)
	if err != nil {
		return p, err
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

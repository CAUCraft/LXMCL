package tasks

import (
	"path/filepath"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

// PrismExport is the exact successfully published local ZIP associated with a task.
type PrismExport struct {
	Path  string            `json:"path"`
	Stamp storage.FileStamp `json:"stamp"`
}

// PrismExportPath returns the recorded path without filesystem access.
func (r Record) PrismExportPath() string {
	p := r.PrismExport
	if r.Action != PrismExportAction || r.Status != Completed || !p.Stamp.Valid() || !filepath.IsAbs(p.Path) || filepath.Clean(p.Path) != p.Path ||
		!storage.ValidFilename(filepath.Base(p.Path)) || !strings.EqualFold(filepath.Ext(p.Path), ".zip") {
		return ""
	}
	return p.Path
}

// LocatePrismExport checks that the original ZIP still occupies its recorded path.
func (r Record) LocatePrismExport() (string, error) {
	filename := r.PrismExportPath()
	if filename == "" || storage.CheckFileStamp(filename, r.PrismExport.Stamp) != nil {
		return "", i18n.Error("prism.export_unavailable")
	}
	return filename, nil
}

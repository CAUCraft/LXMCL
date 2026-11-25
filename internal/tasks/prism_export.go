package tasks

import (
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

// PrismExport is the successfully published local ZIP associated with an export task.
type PrismExport = LocalZIP

// PrismExportPath returns the recorded path without filesystem access.
func (r Record) PrismExportPath() string {
	p := r.PrismExport
	if r.Action != PrismExportAction || r.Status != Completed || !p.Stamp.Valid() {
		return ""
	}
	return p.zipPath()
}

// LocatePrismExport checks that the original ZIP still occupies its recorded path.
func (r Record) LocatePrismExport() (string, error) {
	filename := r.PrismExportPath()
	if filename == "" || storage.CheckFileStamp(filename, r.PrismExport.Stamp) != nil {
		return "", i18n.Error("prism.export_unavailable")
	}
	return filename, nil
}

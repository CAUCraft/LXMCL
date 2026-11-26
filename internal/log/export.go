package log

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

// ExportPreview saves the captured preview without replacing its source log.
// The caller obtains the destination through a native save dialog.
func ExportPreview(source, destination, preview string) error {
	if !filepath.IsAbs(destination) {
		return i18n.Error("log.export_path_required")
	}
	if strings.EqualFold(filepath.Clean(source), filepath.Clean(destination)) || IsInstanceLog(destination) {
		return i18n.Error("log.export_source_conflict")
	}
	if src, err := os.Stat(source); err == nil {
		if dst, err := os.Stat(destination); err == nil && os.SameFile(src, dst) {
			return i18n.Error("log.export_source_conflict")
		}
	}
	return storage.WriteFile(destination, []byte(preview))
}

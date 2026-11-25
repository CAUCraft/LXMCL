package tasks

import (
	"path/filepath"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/storage"
)

// LocalZIP identifies a successfully published ZIP. A zero stamp means the file
// was saved but its identity cannot be verified for later location.
type LocalZIP struct {
	Path  string            `json:"path"`
	Stamp storage.FileStamp `json:"stamp"`
}

func (z LocalZIP) zipPath() string {
	if !filepath.IsAbs(z.Path) || filepath.Clean(z.Path) != z.Path ||
		!storage.ValidFilename(filepath.Base(z.Path)) || !strings.EqualFold(filepath.Ext(z.Path), ".zip") {
		return ""
	}
	return z.Path
}

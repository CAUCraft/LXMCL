package storage

import (
	"path"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

type archivePath struct {
	name      string
	directory bool
}

// ArchivePaths checks the complete tree of portable file and directory names.
// Parents are recorded too: a file cannot also be a directory, and differing
// spellings must not collapse to the same name on case-insensitive filesystems.
type ArchivePaths map[string]archivePath

func (p ArchivePaths) Add(name string, directory bool) error {
	if !ValidPortablePath(name) {
		return i18n.Error("saves.path_invalid", name)
	}
	for name != "." {
		key := strings.ToLower(name)
		entry := archivePath{name, directory}
		if old, exists := p[key]; exists && (!directory || old != entry) {
			return i18n.Error("saves.path_conflict", name)
		}
		p[key] = entry
		name, directory = path.Dir(name), true
	}
	return nil
}

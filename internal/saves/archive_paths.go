package saves

import (
	"path"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

type archivePath struct {
	name      string
	directory bool
}

// Both plans use portable paths so a backup can be restored on another platform.
// Parents are recorded too: a file cannot also be a directory, and differing
// spellings must not collapse to the same name on case-insensitive filesystems.
type archivePaths map[string]archivePath

func (p archivePaths) add(name string, directory bool) error {
	if !storage.ValidPortablePath(name) {
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

package config

import (
	"path/filepath"
	"unicode/utf8"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

// SetWorldPinned changes a local preference for the exact directory within
// this instance. It leaves the instance revision and its saves unchanged.
func (s *State) SetWorldPinned(id int, revision uint64, folder string, pinned bool) error {
	i, ok := s.Find(id)
	if !ok || i.Revision != revision || !filepath.IsLocal(folder) || filepath.Base(folder) != folder || folder == "." || !utf8.ValidString(folder) {
		return i18n.Error("instance.form_target_changed")
	}
	if !pinned {
		delete(s.PinnedWorlds[id], folder)
		if len(s.PinnedWorlds[id]) == 0 {
			delete(s.PinnedWorlds, id)
		}
		return nil
	}
	if s.PinnedWorlds == nil {
		s.PinnedWorlds = make(map[int]map[string]bool)
	}
	if s.PinnedWorlds[id] == nil {
		s.PinnedWorlds[id] = make(map[string]bool)
	}
	s.PinnedWorlds[id][folder] = true
	return nil
}

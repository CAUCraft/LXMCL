package config

import "github.com/CAUCraft/LXMCL/internal/i18n"

// SetModUpdateSkipped changes a launcher preference without invalidating instance tasks.
func (s *State) SetModUpdateSkipped(id int, revision uint64, candidate string, skip bool) error {
	i, ok := s.Find(id)
	if !ok || i.Revision != revision || candidate == "" {
		return i18n.Error("instance.form_target_changed")
	}
	if skip {
		if s.SkippedModUpdates == nil {
			s.SkippedModUpdates = make(map[int]map[string]bool)
		}
		if s.SkippedModUpdates[id] == nil {
			s.SkippedModUpdates[id] = make(map[string]bool)
		}
		s.SkippedModUpdates[id][candidate] = true
	} else {
		delete(s.SkippedModUpdates[id], candidate)
		if len(s.SkippedModUpdates[id]) == 0 {
			delete(s.SkippedModUpdates, id)
		}
	}
	return nil
}

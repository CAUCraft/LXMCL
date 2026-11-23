package config

import "github.com/CAUCraft/LXMCL/internal/i18n"

func (s *State) SetInstanceIcon(id int, revision uint64, previous, png string) error {
	i, ok := s.Find(id)
	if !ok || i.Revision != revision || s.InstanceIcons[id] != previous {
		return i18n.Error("instance.form_target_changed")
	}
	if png == "" {
		delete(s.InstanceIcons, id)
		return nil
	}
	if s.InstanceIcons == nil {
		s.InstanceIcons = make(map[int]string)
	}
	s.InstanceIcons[id] = png
	return nil
}

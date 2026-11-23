package config

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

// GroupNames lists groups that still contain an instance.
func (s State) GroupNames() []string {
	var names []string
	for _, i := range s.Instances {
		if g := s.InstanceGroups[i.ID]; g != "" {
			names = append(names, g)
		}
	}
	slices.Sort(names)
	return slices.Compact(names)
}

func (s *State) SetInstanceGroup(id int, revision uint64, previous, name string) error {
	i, ok := s.Find(id)
	if !ok || i.Revision != revision || s.InstanceGroups[id] != previous {
		return i18n.Error("instance.form_target_changed")
	}
	name = strings.TrimSpace(name)
	if !validGroupName(name) {
		return i18n.Error("instance.group_name_invalid")
	}
	if name == "" {
		delete(s.InstanceGroups, id)
	} else {
		for _, existing := range s.InstanceGroups {
			if strings.EqualFold(existing, name) {
				name = existing
				break
			}
		}
		if s.InstanceGroups == nil {
			s.InstanceGroups = make(map[int]string)
		}
		s.InstanceGroups[id] = name
	}
	s.clearMissingGroupFilter()
	return nil
}

func validGroupName(name string) bool {
	return utf8.RuneCountInString(name) <= 48 && !strings.ContainsFunc(name, unicode.IsControl)
}

func (s *State) RenameInstanceGroup(previous, name string) error {
	name = strings.TrimSpace(name)
	if name == "" || !validGroupName(name) {
		return i18n.Error("instance.group_name_invalid")
	}
	groups := s.GroupNames()
	if !slices.Contains(groups, previous) {
		return i18n.Error("instance.form_target_changed")
	}
	for _, g := range groups {
		if g != previous && strings.EqualFold(g, name) {
			return i18n.Error("instance.group_name_taken")
		}
	}
	for id, g := range s.InstanceGroups {
		if g == previous {
			s.InstanceGroups[id] = name
		}
	}
	if s.LibraryGroup != nil && *s.LibraryGroup == previous {
		s.LibraryGroup = &name
	}
	return nil
}

func (s *State) clearMissingGroupFilter() {
	if s.LibraryGroup != nil && *s.LibraryGroup != "" && !slices.Contains(s.GroupNames(), *s.LibraryGroup) {
		s.LibraryGroup = nil
	}
}

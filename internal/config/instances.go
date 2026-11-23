package config

import (
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

func (s State) Find(id int) (instance.Instance, bool) {
	for _, i := range s.Instances {
		if i.ID == id {
			return i, true
		}
	}
	return instance.Instance{}, false
}

func (s *State) Create(name, version string) error {
	name, err := s.checkName(0, name)
	if err != nil {
		return err
	}
	if !instance.SupportedRelease(version) {
		return i18n.Error("instance.release_required")
	}
	if !filepath.IsAbs(s.Settings.GameRoot) {
		return i18n.Error("instance.game_root_required")
	}
	id := s.nextInstanceID()
	if id == math.MaxInt {
		return i18n.Error("instance.id_exhausted")
	}
	dir := filepath.Join(s.Settings.GameRoot, fmt.Sprintf("instance-%d", id))
	for _, other := range s.Instances {
		if filepath.Clean(other.GameDir) == dir {
			return i18n.Error("instance.directory_occupied")
		}
	}
	i := instance.Instance{ID: id, Name: name, Version: version, Loader: instance.LoaderVanilla, Revision: 1, GameDir: dir}
	s.NextID = id + 1
	s.Instances = append(s.Instances, i)
	s.Selected = i.ID
	return nil
}

func (s *State) Duplicate(id int, revision uint64, name, group string) (instance.Instance, error) {
	source, ok := s.Find(id)
	if !ok || source.Revision != revision {
		return instance.Instance{}, i18n.Error("instance.duplicate_target_changed")
	}
	if err := source.Validate(); err != nil {
		return instance.Instance{}, err
	}
	if group != "" && !slices.Contains(s.GroupNames(), group) {
		return instance.Instance{}, i18n.Error("instance.group_missing")
	}
	selected := s.Selected
	if err := s.Create(name, source.Version); err != nil {
		return instance.Instance{}, err
	}
	s.Selected = selected
	duplicate := &s.Instances[len(s.Instances)-1]
	if group != "" {
		s.InstanceGroups[duplicate.ID] = group
	}
	duplicate.Launch = source.Launch
	duplicate.Loader, duplicate.LoaderVersion = source.Loader, source.LoaderVersion
	return *duplicate, nil
}

func (s State) checkName(id int, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 48 {
		return "", i18n.Error("instance.name_invalid")
	}
	for _, i := range s.Instances {
		if i.ID != id && strings.EqualFold(i.Name, name) {
			return "", i18n.Error("instance.name_conflict")
		}
	}
	return name, nil
}

func (s *State) Remove(id int) {
	delete(s.InstanceNotes, id)
	delete(s.InstanceIcons, id)
	delete(s.InstanceGroups, id)
	delete(s.PinnedInstances, id)
	s.clearMissingGroupFilter()
	s.Instances = slices.DeleteFunc(s.Instances, func(i instance.Instance) bool { return i.ID == id })
	if s.Selected == id {
		s.Selected = 0
		if len(s.Instances) != 0 {
			s.Selected = s.Instances[0].ID
		}
	}
}

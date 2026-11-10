package launcher

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

func (s State) Find(id int) (Instance, bool) {
	for _, i := range s.Instances {
		if i.ID == id {
			return i, true
		}
	}
	return Instance{}, false
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
	i := Instance{ID: s.NextID, Name: name, Version: version, Loader: "Vanilla", Revision: 1,
		GameDir: filepath.Join(s.Settings.GameRoot, fmt.Sprintf("instance-%d", s.NextID))}
	s.NextID++
	s.Instances = append(s.Instances, i)
	s.Selected = i.ID
	return nil
}

func (s *State) Duplicate(id int, revision uint64, name string) (Instance, error) {
	source, ok := s.Find(id)
	if !ok || source.Revision != revision {
		return Instance{}, i18n.Error("instance.duplicate_target_changed")
	}
	if err := source.Validate(); err != nil {
		return Instance{}, err
	}
	selected := s.Selected
	if err := s.Create(name, source.Version); err != nil {
		return Instance{}, err
	}
	s.Selected = selected
	duplicate := &s.Instances[len(s.Instances)-1]
	duplicate.Launch = source.Launch
	duplicate.Loader, duplicate.LoaderVersion = source.Loader, source.LoaderVersion
	return *duplicate, nil
}

func (s *State) ConfigureLoader(id int, revision uint64, fabricVersion string) error {
	kind := LoaderVanilla
	if fabricVersion != "" {
		kind = LoaderFabric
	}
	return s.ConfigureModLoader(id, revision, kind, fabricVersion)
}

func (s *State) ConfigureModLoader(id int, revision uint64, kind, version string) error {
	for n := range s.Instances {
		i := &s.Instances[n]
		if i.ID != id || i.Revision != revision {
			continue
		}
		next := *i
		loader, number, err := instance.NormalizeLoader(i.Version, kind, version)
		if err != nil {
			return err
		}
		next.Loader, next.LoaderVersion = loader, number
		if err := next.Validate(); err != nil {
			return err
		}
		if next.Loader != i.Loader || next.LoaderVersion != i.LoaderVersion {
			next.Revision++
			*i = next
		}
		return nil
	}
	return i18n.Error("instance.loader_target_changed")
}

func (s *State) Rename(id int, revision uint64, name string) error {
	name, err := s.checkName(id, name)
	if err != nil {
		return err
	}
	for n := range s.Instances {
		i := &s.Instances[n]
		if i.ID == id && i.Revision == revision {
			if i.Name != name {
				i.Name = name
				i.Revision++
			}
			return nil
		}
	}
	return i18n.Error("instance.form_target_changed")
}

func (s *State) ChangeVersion(id int, revision uint64, version string) error {
	for n := range s.Instances {
		i := &s.Instances[n]
		if i.ID != id || i.Revision != revision {
			continue
		}
		next := *i
		next.Version = version
		if err := next.Validate(); err != nil {
			return err
		}
		if i.Version != version {
			i.Version = version
			i.Revision++
		}
		return nil
	}
	return i18n.Error("instance.form_target_changed")
}

func (s *State) ConfigureLaunch(id int, revision uint64, opts LaunchOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	for n := range s.Instances {
		i := &s.Instances[n]
		if i.ID == id && i.Revision == revision {
			if i.Launch != opts {
				i.Launch = opts
				i.Revision++
			}
			return nil
		}
	}
	return i18n.Error("instance.settings_target_changed")
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
	s.Instances = slices.DeleteFunc(s.Instances, func(i Instance) bool { return i.ID == id })
	if s.Selected == id {
		s.Selected = 0
		if len(s.Instances) != 0 {
			s.Selected = s.Instances[0].ID
		}
	}
}

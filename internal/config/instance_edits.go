package config

import (
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

func (s *State) ConfigureLoader(id int, revision uint64, fabricVersion string) error {
	kind := instance.LoaderVanilla
	if fabricVersion != "" {
		kind = instance.LoaderFabric
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

func (s *State) ConfigureLaunch(id int, revision uint64, opts instance.LaunchOptions) error {
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

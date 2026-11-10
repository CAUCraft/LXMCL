package config

import "github.com/CAUCraft/LXMCL/internal/instance"

// PrepareInstallation returns an independent draft and the new installation
// target. A valid prior selection is retained. Failure returns the original
// snapshot without allocating an ID or changing shared slices. No files are made.
func (s State) PrepareInstallation(name, version, loader, loaderVersion string) (State, instance.Instance, error) {
	next := s.Clone()
	if err := next.Create(name, version); err != nil {
		return s, instance.Instance{}, err
	}
	i := next.Instances[len(next.Instances)-1]
	if err := next.ConfigureModLoader(i.ID, i.Revision, loader, loaderVersion); err != nil {
		return s, instance.Instance{}, err
	}
	i = next.Instances[len(next.Instances)-1]
	if _, ok := s.Find(s.Selected); ok {
		next.Selected = s.Selected
	}
	return next, i, nil
}

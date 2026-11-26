package content

import (
	"cmp"
	"context"
	"slices"

	"github.com/CAUCraft/LXMCL/internal/instance"
)

type ResourcePackState uint8

const (
	PackAbsent ResourcePackState = iota
	PackPresent
	PackDisabled
	PackDuplicate
)

type ResourcePackMatch struct {
	Name           string
	Directory      bool
	Current, Other ResourcePackState
}

type ResourcePackScanError struct {
	InstanceID int
	Err        error
}

func (e *ResourcePackScanError) Error() string { return e.Err.Error() }
func (e *ResourcePackScanError) Unwrap() error { return e.Err }

// CompareResourcePacks pairs exact inventory names and types. For a ZIP, the
// inventory excludes the launcher's .disabled suffix from the name.
// Neither ZIP contents nor the game's options are read.
func CompareResourcePacks(ctx context.Context, current, other instance.Instance) ([]ResourcePackMatch, error) {
	type key struct {
		name      string
		directory bool
	}
	pairs := make(map[key][2]ResourcePackState)
	for side, i := range []instance.Instance{current, other} {
		files, err := list(ctx, i, ResourcePacks, true, true)
		if err != nil {
			return nil, &ResourcePackScanError{InstanceID: i.ID, Err: err}
		}
		for _, f := range files {
			k := key{name: f.Name, directory: f.Directory}
			pair := pairs[k]
			switch {
			case pair[side] != PackAbsent:
				pair[side] = PackDuplicate
			case !f.Enabled:
				pair[side] = PackDisabled
			default:
				pair[side] = PackPresent
			}
			pairs[k] = pair
		}
	}
	matches := make([]ResourcePackMatch, 0, len(pairs))
	for k, p := range pairs {
		matches = append(matches, ResourcePackMatch{Name: k.name, Directory: k.directory, Current: p[0], Other: p[1]})
	}
	slices.SortFunc(matches, func(a, b ResourcePackMatch) int {
		if n := cmp.Compare(a.Name, b.Name); n != 0 {
			return n
		}
		if a.Directory == b.Directory {
			return 0
		}
		if a.Directory {
			return -1
		}
		return 1
	})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return matches, nil
}

package content

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/instance"
)

type ModState struct {
	Version string
	Enabled bool
}

type ModDifference struct {
	Name          string
	ByFilename    bool
	Before, After *ModState
}

func (d ModDifference) VersionChanged() bool {
	return d.Before != nil && d.After != nil && d.Before.Version != "" && d.After.Version != "" && d.Before.Version != d.After.Version
}

type ModComparison struct {
	Rows            []ModDifference
	Ambiguous       []string
	UnknownVersions int
}

// CompareMods scans both local directories with the same loader-specific inventory used by the resource page.
func CompareMods(ctx context.Context, base, other instance.Instance) (ModComparison, error) {
	type key struct {
		name string
		file bool
	}
	type pair [2][]ModState
	index := make(map[key]*pair)
	add := func(k key, side int, state ModState) {
		p := index[k]
		if p == nil {
			p = new(pair)
			index[k] = p
		}
		p[side] = append(p[side], state)
	}
	for side, i := range []instance.Instance{base, other} {
		files, err := Inventory(ctx, i, Mods)
		if err != nil {
			return ModComparison{}, fmt.Errorf("%s：%w", i.Name, err)
		}
		for _, f := range files {
			if len(f.mods) == 0 {
				add(key{name: f.Name, file: true}, side, ModState{Enabled: f.Enabled})
			}
			for _, m := range f.mods {
				version := strings.TrimSpace(m.version)
				if strings.Contains(version, "${") {
					version = ""
				}
				add(key{name: m.id}, side, ModState{Version: version, Enabled: f.Enabled})
			}
		}
	}
	var result ModComparison
	for k, p := range index {
		if len(p[0]) > 1 || len(p[1]) > 1 {
			result.Ambiguous = append(result.Ambiguous, k.name)
			continue
		}
		d := ModDifference{Name: k.name, ByFilename: k.file}
		if len(p[0]) == 1 {
			d.Before = &p[0][0]
		}
		if len(p[1]) == 1 {
			d.After = &p[1][0]
		}
		if d.Before != nil && d.After != nil {
			if d.Before.Version == "" || d.After.Version == "" {
				result.UnknownVersions++
			}
			if !d.VersionChanged() && d.Before.Enabled == d.After.Enabled {
				continue
			}
		}
		result.Rows = append(result.Rows, d)
	}
	slices.Sort(result.Ambiguous)
	slices.SortFunc(result.Rows, func(a, b ModDifference) int {
		if n := cmp.Compare(a.Name, b.Name); n != 0 {
			return n
		}
		if a.ByFilename == b.ByFilename {
			return 0
		}
		if a.ByFilename {
			return 1
		}
		return -1
	})
	return result, nil
}

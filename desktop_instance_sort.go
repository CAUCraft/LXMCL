package main

import (
	"cmp"
	"slices"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/field"
)

const (
	sortName     = "name"
	sortRecent   = "last_played"
	sortDuration = "playtime"
)

func (a *desktop) librarySortPicker(c *ui.Context) {
	options := []field.SelectOption{
		{Value: "", Label: "默认顺序"}, {Value: sortName, Label: "按名称"},
		{Value: sortRecent, Label: "最近游玩"}, {Value: sortDuration, Label: "游玩时长"},
	}
	mode := a.state.LibrarySort
	catalogPicker(c, &mode, options, "实例排序", "", a.busy).Width(112)
	if mode == a.state.LibrarySort {
		return
	}
	next := a.state.Clone()
	next.LibrarySort = mode
	a.save(next, 0, func(err error) {
		if err != nil {
			a.message("保存实例排序失败", err)
			return
		}
		a.scroll.Y = 0
	})
}

func (a *desktop) libraryInstances() []launcher.Instance {
	query := strings.ToLower(strings.TrimSpace(a.query))
	var rows []launcher.Instance
	for _, i := range a.state.Instances {
		if matchesInstanceSearch(i, query) && a.matchesCacheFilter(i) && a.matchesGroup(i.ID) {
			rows = append(rows, i)
		}
	}
	switch a.state.LibrarySort {
	case sortName:
		slices.SortStableFunc(rows, func(x, y launcher.Instance) int {
			return strings.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name))
		})
	case sortRecent:
		slices.SortStableFunc(rows, func(x, y launcher.Instance) int {
			return a.playtimes[y.ID].LastPlayed.Compare(a.playtimes[x.ID].LastPlayed)
		})
	case sortDuration:
		slices.SortStableFunc(rows, func(x, y launcher.Instance) int {
			return cmp.Compare(a.playtimes[y.ID].TotalMillis(), a.playtimes[x.ID].TotalMillis())
		})
	}
	return rows
}

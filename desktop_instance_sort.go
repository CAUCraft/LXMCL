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

func (a *desktop) setInstancePinned(i launcher.Instance, pinned bool) {
	next := a.state.Clone()
	if current, ok := next.Find(i.ID); !ok || current.Revision != i.Revision {
		a.message("实例已更改，请重新选择", nil)
		return
	}
	if pinned {
		if next.PinnedInstances == nil {
			next.PinnedInstances = make(map[int]bool)
		}
		next.PinnedInstances[i.ID] = true
	} else {
		delete(next.PinnedInstances, i.ID)
	}
	a.save(next, 0, func(err error) {
		if err != nil {
			a.message("保存置顶状态失败", err)
		}
	})
}

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
		if matchesInstanceSearch(i, query) && a.matchesCacheFilter(i) && a.matchesGroup(i.ID) &&
			a.matchesLibraryLoader(i) && a.matchesLibraryVersion(i) {
			rows = append(rows, i)
		}
	}
	return a.orderLibraryInstances(rows)
}

func (a *desktop) orderLibraryInstances(rows []launcher.Instance) []launcher.Instance {
	if a.state.LibrarySort == "" && len(a.state.PinnedInstances) == 0 {
		return rows
	}
	slices.SortStableFunc(rows, func(x, y launcher.Instance) int {
		if px, py := a.state.PinnedInstances[x.ID], a.state.PinnedInstances[y.ID]; px != py {
			if px {
				return -1
			}
			return 1
		}
		switch a.state.LibrarySort {
		case sortName:
			return strings.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name))
		case sortRecent:
			return a.playtimes[y.ID].LastPlayed.Compare(a.playtimes[x.ID].LastPlayed)
		case sortDuration:
			return cmp.Compare(a.playtimes[y.ID].TotalMillis(), a.playtimes[x.ID].TotalMillis())
		default:
			return 0
		}
	})
	return rows
}

package main

import (
	"maps"
	"slices"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/field"
)

func (a *desktop) matchesLibraryVersion(i launcher.Instance) bool {
	return matchesInstanceVersion(i, a.libraryVersion)
}

func matchesInstanceVersion(i launcher.Instance, version string) bool {
	return version == "" || i.Version == version
}

func (a *desktop) libraryVersionPicker(c *ui.Context) {
	versions := make(map[string]bool, len(a.state.Instances))
	for _, i := range a.state.Instances {
		if i.Version != "" {
			versions[i.Version] = true
		}
	}
	options := []field.SelectOption{{Value: "", Label: "全部版本"}}
	for _, version := range slices.Sorted(maps.Keys(versions)) {
		options = append(options, field.SelectOption{Value: "version:" + version, Label: version})
	}
	key, fallback := "", ""
	switch {
	case a.libraryVersion != "":
		key = "version:" + a.libraryVersion
		if !versions[a.libraryVersion] {
			fallback = a.libraryVersion + " · 无实例"
		}
	case a.state.LibraryVersion != "":
		key, fallback = "revealed", "临时查看全部"
	}
	selected := key
	catalogPicker(c, &selected, options, "筛选实例版本", fallback, a.busy).Width(112)
	if selected != key {
		version, _ := strings.CutPrefix(selected, "version:")
		next := a.state.Clone()
		next.LibraryVersion = version
		a.saveLibraryFilters(next, a.libraryGroup, a.libraryLoader, version)
	}
}

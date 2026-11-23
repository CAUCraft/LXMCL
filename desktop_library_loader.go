package main

import (
	"strings"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/field"
)

const unknownLibraryLoader = "unknown"

func libraryLoaderKey(i launcher.Instance) string {
	if i.Vanilla() {
		return launcher.LoaderVanilla
	}
	for _, loader := range installLoaders {
		if strings.EqualFold(i.Loader, loader) {
			return loader
		}
	}
	return unknownLibraryLoader
}

func (a *desktop) matchesLibraryLoader(i launcher.Instance) bool {
	return a.libraryLoader == "" || a.libraryLoader == libraryLoaderKey(i)
}

func (a *desktop) libraryLoaderPicker(c *ui.Context) {
	options := []field.SelectOption{{Value: "", Label: "全部加载器"}}
	for _, loader := range installLoaders {
		label := loader
		if loader == launcher.LoaderVanilla {
			label = "原版"
		}
		options = append(options, field.SelectOption{Value: loader, Label: label})
	}
	options = append(options, field.SelectOption{Value: unknownLibraryLoader, Label: "未识别"})
	key, fallback := a.libraryLoader, ""
	if key == "" && a.state.LibraryLoader != "" {
		key, fallback = "revealed", "临时查看全部"
	}
	selected := key
	field.Select(c, &selected, field.SelectProps{Options: options, Placeholder: fallback, Disabled: a.busy}).
		Label("筛选实例加载器").Width(116)
	if selected != key {
		next := a.state.Clone()
		next.LibraryLoader = selected
		a.saveLibraryFilters(next, a.libraryGroup, selected)
	}
}

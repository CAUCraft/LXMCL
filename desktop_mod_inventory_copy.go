package main

import (
	"cmp"
	"slices"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/launcher"
)

func installedModReport(files []launcher.ContentFile) string {
	lines := make([]string, 0, len(files))
	for _, file := range files {
		name := cmp.Or(shareField(file.Name), "名称未知")
		mods := file.ModMetadata()
		if len(mods) == 0 {
			lines = append(lines, name+" · "+modSide(&content.ModState{Enabled: file.Enabled}))
		}
		for _, mod := range mods {
			title := cmp.Or(shareField(mod.Name), shareField(mod.ID), name)
			state := content.ModState{Version: shareField(mod.Version), Enabled: file.Enabled}
			lines = append(lines, title+" · "+modSide(&state))
		}
	}
	slices.Sort(lines)
	return strings.Join(lines, "\n")
}

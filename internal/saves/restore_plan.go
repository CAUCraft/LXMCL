package saves

import (
	"archive/zip"
	"context"
	"path"
	"sort"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

type restorePlan struct {
	entries []*zip.File
	worlds  []string
}

// planRestore checks the whole ZIP before opening or creating the game directory.
func planRestore(ctx context.Context, entries []*zip.File) (restorePlan, error) {
	var plan restorePlan
	if len(entries) == 0 || len(entries) > maxSaveEntries {
		return plan, i18n.Error("saves.archive_entry_count_invalid")
	}
	paths := make(archivePaths)
	worlds := make(map[string]bool)
	var size uint64
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return plan, err
		}
		name := strings.TrimSuffix(entry.Name, "/")
		directory := entry.FileInfo().IsDir()
		if !entry.Mode().IsRegular() && !directory {
			return plan, i18n.Error("saves.archive_path_or_type_invalid", entry.Name)
		}
		if err := paths.add(name, directory); err != nil {
			return plan, err
		}
		world, rest, nested := strings.Cut(name, "/")
		if !nested && !directory {
			return plan, i18n.Error("saves.archive_world_directory_required")
		}
		worlds[world] = worlds[world] || (rest == "level.dat" && entry.Mode().IsRegular())
		if entry.UncompressedSize64 > maxSaveBytes-size {
			return plan, i18n.Error("saves.archive_expanded_too_large")
		}
		size += entry.UncompressedSize64
		if directory || path.Base(name) != "session.lock" {
			plan.entries = append(plan.entries, entry)
		}
	}
	for name, valid := range worlds {
		if !valid {
			return plan, i18n.Error("saves.archive_level_dat_missing", name)
		}
		plan.worlds = append(plan.worlds, name)
	}
	sort.Strings(plan.worlds)
	return plan, nil
}

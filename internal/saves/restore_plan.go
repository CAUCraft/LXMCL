package saves

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

type restoreEntry struct {
	file *zip.File
	name string
}

type restorePlan struct {
	entries   []restoreEntry
	worlds    []string
	signature [sha256.Size]byte
}

// planRestore checks the whole ZIP before opening or creating the game directory.
func planRestore(ctx context.Context, source string, entries []*zip.File) (restorePlan, error) {
	var plan restorePlan
	if len(entries) == 0 || len(entries) > maxSaveEntries {
		return plan, i18n.Error("saves.archive_entry_count_invalid")
	}
	prefix := ""
	if slices.ContainsFunc(entries, func(f *zip.File) bool { return f.Name == "level.dat" && f.Mode().IsRegular() }) {
		base := filepath.Base(source)
		prefix = strings.TrimSuffix(base, filepath.Ext(base))
		if !storage.ValidPortablePath(prefix) {
			prefix = "World"
		}
	}
	paths := make(archivePaths)
	worlds := make(map[string]bool)
	index := sha256.New()
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
		// The validated names, types, sizes and CRCs describe the preview without
		// reading gigabytes of world data just to populate a selection dialog.
		fmt.Fprintf(index, "%q %t %d %08x\n", name, directory, entry.UncompressedSize64, entry.CRC32)
		if prefix != "" {
			if path.Base(name) == "level.dat" && strings.Count(name, "/") == 1 && !directory {
				return plan, i18n.Error("saves.archive_mixed_worlds")
			}
			name = prefix + "/" + name
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
			plan.entries = append(plan.entries, restoreEntry{file: entry, name: name})
		}
	}
	for name, valid := range worlds {
		if !valid {
			return plan, i18n.Error("saves.archive_level_dat_missing", name)
		}
		plan.worlds = append(plan.worlds, name)
	}
	copy(plan.signature[:], index.Sum(nil))
	sort.Strings(plan.worlds)
	return plan, nil
}

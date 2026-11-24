package saves

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

// Restore imports ZIPs with level.dat at the root or inside top-level world folders.
// Imported worlds receive a new suffix if their name is already taken. Callers
// must serialize changes to the same instance. On publication failure, the count
// reports worlds already restored.
func Restore(ctx context.Context, gameDir, source string) (int, error) {
	return restore(ctx, gameDir, source, nil, "")
}

// WorldStats describes restorable regular files from the validated ZIP
// directory. UncompressedBytes is declared by the archive.
type WorldStats struct {
	FileCount         int
	UncompressedBytes uint64
}

// Preview holds validated world names and statistics and a ZIP directory
// signature. Reading a preview never opens or creates the game directory.
type Preview struct {
	source    string
	worlds    []string
	stats     map[string]WorldStats
	signature [sha256.Size]byte
}

func (p Preview) Worlds() []string { return slices.Clone(p.worlds) }

// WorldStats returns a copy of the available statistics, keyed by world name.
func (p Preview) WorldStats() map[string]WorldStats { return maps.Clone(p.stats) }

func Inspect(ctx context.Context, source string) (Preview, error) {
	z, plan, err := openRestorePlan(ctx, source)
	if err != nil {
		return Preview{}, err
	}
	defer z.Close()
	stats := make(map[string]WorldStats, len(plan.worlds))
	for _, entry := range plan.entries {
		if err := ctx.Err(); err != nil {
			return Preview{}, err
		}
		if !entry.file.Mode().IsRegular() {
			continue
		}
		world, _, _ := strings.Cut(entry.name, "/")
		current := stats[world]
		current.FileCount++
		current.UncompressedBytes += entry.file.UncompressedSize64
		stats[world] = current
	}
	return Preview{source: source, worlds: plan.worlds, stats: stats, signature: plan.signature}, nil
}

// Restore revalidates the ZIP before extracting one world, or all worlds when
// world is empty. CRCs are checked by the ZIP reader during extraction.
func (p Preview) Restore(ctx context.Context, gameDir, world string) (int, error) {
	return restore(ctx, gameDir, p.source, &p, world)
}

func openRestorePlan(ctx context.Context, source string) (*zip.ReadCloser, restorePlan, error) {
	if err := ctx.Err(); err != nil {
		return nil, restorePlan{}, err
	}
	z, err := zip.OpenReader(source)
	if err != nil {
		return nil, restorePlan{}, err
	}
	plan, err := planRestore(ctx, source, z.File)
	if err != nil {
		z.Close()
		return nil, restorePlan{}, err
	}
	return z, plan, nil
}

func restore(ctx context.Context, gameDir, source string, preview *Preview, world string) (int, error) {
	z, plan, err := openRestorePlan(ctx, source)
	if err != nil {
		return 0, err
	}
	defer z.Close()
	if preview != nil && preview.signature != plan.signature {
		return 0, i18n.Error("saves.archive_changed")
	}
	if world != "" {
		if _, ok := slices.BinarySearch(plan.worlds, world); !ok {
			return 0, i18n.Error("saves.world_missing", world)
		}
		plan.worlds = []string{world}
		plan.entries = slices.DeleteFunc(plan.entries, func(f restoreEntry) bool {
			top, _, _ := strings.Cut(f.name, "/")
			return top != world
		})
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	root, err := savesRoot(gameDir, true)
	if err != nil {
		return 0, err
	}
	defer root.Close()
	stage := ".lxmcl-restore-" + rand.Text()
	if err := root.Mkdir(stage, 0700); err != nil {
		return 0, err
	}
	defer root.RemoveAll(stage)
	staged, err := root.OpenRoot(stage)
	if err != nil {
		return 0, err
	}
	if err := errors.Join(extractArchive(ctx, staged, plan.entries), staged.Close()); err != nil {
		return 0, err
	}
	n, err := publishWorlds(ctx, root, stage, plan.worlds)
	if err != nil && n > 0 {
		return n, i18n.Error("saves.restore_partial", n, err)
	}
	return n, err
}

func publishWorlds(ctx context.Context, root *os.Root, stage string, worlds []string) (int, error) {
	// A generated copy name must not claim another imported world's original name.
	reserved := make(map[string]bool, len(worlds))
	for _, name := range worlds {
		reserved[strings.ToLower(name)] = true
	}
	for n, world := range worlds {
		dst := world
		for suffix := 1; ; suffix++ {
			if err := ctx.Err(); err != nil {
				return n, err
			}
			if suffix > 1 {
				dst = fmt.Sprintf("%s (%d)", world, suffix)
				if reserved[strings.ToLower(dst)] {
					continue
				}
			}
			if _, err := root.Lstat(dst); errors.Is(err, fs.ErrNotExist) {
				break
			} else if err != nil {
				return n, err
			}
		}
		if err := root.Rename(path.Join(stage, world), dst); err != nil {
			return n, err
		}
	}
	return len(worlds), nil
}

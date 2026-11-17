package saves

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
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

// Preview holds the validated world names and a signature of the ZIP directory.
// Reading a preview never opens or creates the destination game directory.
type Preview struct {
	source    string
	worlds    []string
	signature [sha256.Size]byte
}

func (p Preview) Worlds() []string { return slices.Clone(p.worlds) }

func Inspect(ctx context.Context, source string) (Preview, error) {
	z, plan, err := openRestorePlan(ctx, source)
	if err != nil {
		return Preview{}, err
	}
	defer z.Close()
	return Preview{source: source, worlds: plan.worlds, signature: plan.signature}, nil
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
	for n, world := range worlds {
		dst := world
		for suffix := 2; ; suffix++ {
			if err := ctx.Err(); err != nil {
				return n, err
			}
			if _, err := root.Lstat(dst); errors.Is(err, fs.ErrNotExist) {
				break
			} else if err != nil {
				return n, err
			}
			dst = fmt.Sprintf("%s (%d)", world, suffix)
		}
		if err := root.Rename(path.Join(stage, world), dst); err != nil {
			return n, err
		}
	}
	return len(worlds), nil
}

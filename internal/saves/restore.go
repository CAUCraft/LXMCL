package saves

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

// Restore imports ZIPs whose top-level folders contain level.dat.
// Imported worlds receive a new suffix if their name is already taken. Callers
// must serialize changes to the same instance. On publication failure, the count
// reports worlds already restored.
func Restore(ctx context.Context, gameDir, source string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	z, err := zip.OpenReader(source)
	if err != nil {
		return 0, err
	}
	defer z.Close()
	plan, err := planRestore(ctx, z.File)
	if err != nil {
		return 0, err
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
	return publishWorlds(ctx, root, stage, plan.worlds)
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
			return n, i18n.Error("saves.restore_partial", n, err)
		}
	}
	return len(worlds), nil
}

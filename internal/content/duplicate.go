package content

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/CAUCraft/LXMCL/internal/tasks"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

// Values distinguish directory trees from individual files.
var setupPaths = map[string]bool{
	"mods": true, "resourcepacks": true, "shaderpacks": true, "config": true,
	"defaultconfigs": true, "kubejs": true, "scripts": true,
	"options.txt": false, "optionsof.txt": false, "optionsshaders.txt": false, "servers.dat": false,
}

// DuplicateSetup copies resources and game settings into a new, independent
// directory. Worlds, screenshots and launcher account files are not copied.
// Callers stop game writes and serialize configuration commits. A failed commit
// rolls the new directory back; it never removes or modifies the source.
func DuplicateSetup(ctx context.Context, source, destination string, commit func() error, report func(tasks.Progress)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !filepath.IsAbs(source) || !filepath.IsAbs(destination) || commit == nil {
		return i18n.Error("instance.duplicate_target_changed")
	}
	src, err := os.OpenRoot(source)
	if err != nil {
		return err
	}
	defer src.Close()
	parent, name := filepath.Dir(destination), filepath.Base(destination)
	sourcePath, err := filepath.EvalSymlinks(source)
	if err != nil {
		return err
	}
	// A configured game root may not exist yet. Check its nearest existing
	// ancestor before creating directories, including through a directory link.
	ancestor := parent
	for {
		_, err := os.Lstat(ancestor)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrNotExist) || filepath.Dir(ancestor) == ancestor {
			return err
		}
		ancestor = filepath.Dir(ancestor)
	}
	parentPath, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return err
	}
	if relative, err := filepath.Rel(sourcePath, parentPath); err == nil && (relative == "." || filepath.IsLocal(relative)) {
		return i18n.Error("instance.duplicate_nested_directory")
	}
	if err := os.MkdirAll(parent, 0755); err != nil {
		return err
	}
	base, err := os.OpenRoot(parent)
	if err != nil {
		return err
	}
	defer base.Close()
	checkDestination := func() error {
		if _, err := base.Lstat(name); !errors.Is(err, os.ErrNotExist) {
			if err == nil {
				return i18n.Error("instance.duplicate_directory_exists", destination)
			}
			return err
		}
		return nil
	}
	if err := checkDestination(); err != nil {
		return err
	}
	stage := ".lxmcl-copy-" + rand.Text()
	if err := base.Mkdir(stage, 0700); err != nil {
		return err
	}
	defer base.RemoveAll(stage)
	dst, err := base.OpenRoot(stage)
	if err != nil {
		return err
	}
	err = copySetup(ctx, src, dst, report)
	if err := errors.Join(err, dst.Close()); err != nil {
		return err
	}
	if report != nil {
		report(tasks.Progress{Stage: i18n.Text("progress.save_instance")})
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := checkDestination(); err != nil {
		return err
	}
	if err := base.Rename(stage, name); err != nil {
		return err
	}
	if err := commit(); err != nil {
		return errors.Join(err, base.Rename(name, stage))
	}
	return nil
}

func copySetup(ctx context.Context, source, destination *os.Root, report func(tasks.Progress)) error {
	count := 0
	return fs.WalkDir(source.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil || name == "." {
			return walkErr
		}
		if !strings.Contains(name, "/") {
			directory, include := setupPaths[name]
			if !include {
				if entry.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if entry.IsDir() != directory {
				return i18n.Error("instance.duplicate_file_unsupported", name)
			}
		}
		if !storage.ValidPortablePath(name) || entry.Type()&os.ModeSymlink != 0 || (!entry.IsDir() && !entry.Type().IsRegular()) {
			return i18n.Error("instance.duplicate_file_unsupported", name)
		}
		path := filepath.FromSlash(name)
		if entry.IsDir() {
			return destination.MkdirAll(path, 0755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if report != nil {
			report(tasks.Progress{Stage: i18n.Text("progress.copy_instance_file", name), Done: count})
		}
		if err := copySetupFile(ctx, source, destination, path, info); err != nil {
			return i18n.Error("instance.duplicate_file_failed", name, err)
		}
		count++
		return nil
	})
}

func copySetupFile(ctx context.Context, source, destination *os.Root, name string, expected fs.FileInfo) error {
	in, err := source.Open(name)
	if err != nil {
		return err
	}
	defer in.Close()
	before, err := in.Stat()
	if err != nil {
		return err
	}
	if !before.Mode().IsRegular() || !os.SameFile(expected, before) || before.Size() != expected.Size() || !before.ModTime().Equal(expected.ModTime()) {
		return i18n.Error("instance.duplicate_source_changed")
	}
	out, err := destination.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(out, io.LimitReader(transfer.Reader(ctx, in), before.Size()+1))
	after, statErr := in.Stat()
	if statErr == nil && (n != before.Size() || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime())) {
		statErr = i18n.Error("instance.duplicate_source_changed")
	}
	err = errors.Join(copyErr, statErr, out.Sync(), out.Close())
	if err != nil {
		return err
	}
	return destination.Chtimes(name, before.ModTime(), before.ModTime())
}

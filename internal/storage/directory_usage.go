package storage

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
)

type FileUsage struct {
	Files uint64
	Bytes uint64
}

// DirectoryFileUsage sums logical sizes of ordinary files below directory.
// Links and the shared cache are skipped. An incomplete walk returns no totals.
func DirectoryFileUsage(ctx context.Context, directory, sharedCache string) (FileUsage, error) {
	if err := ctx.Err(); err != nil {
		return FileUsage{}, err
	}
	if !filepath.IsAbs(directory) {
		return FileUsage{}, fs.ErrInvalid
	}
	directory = filepath.Clean(directory)
	parent, err := filepath.EvalSymlinks(filepath.Dir(directory))
	if err != nil {
		return FileUsage{}, err
	}
	// Resolve the configured parent, but require the instance entry itself to
	// be a real directory.
	directory = filepath.Join(parent, filepath.Base(directory))
	expected, err := os.Lstat(directory)
	if err != nil {
		return FileUsage{}, err
	}
	if expected.Mode().Type() != fs.ModeDir {
		return FileUsage{}, fs.ErrInvalid
	}

	cache, cacheInfo, err := usageCache(sharedCache)
	if err != nil {
		return FileUsage{}, err
	}
	if cache != "" {
		relative, err := filepath.Rel(cache, directory)
		if (err == nil && filepath.IsLocal(relative)) || (cacheInfo != nil && os.SameFile(expected, cacheInfo)) {
			return FileUsage{}, fs.ErrInvalid
		}
	}

	root, err := os.OpenRoot(directory)
	if err != nil {
		return FileUsage{}, err
	}
	defer root.Close()
	var usage FileUsage
	if err := countDirectoryFiles(ctx, root, expected, cache, cacheInfo, &usage); err != nil {
		return FileUsage{}, err
	}
	current, err := os.Lstat(directory)
	if err != nil {
		return FileUsage{}, err
	}
	if current.Mode().Type() != fs.ModeDir || !os.SameFile(expected, current) {
		return FileUsage{}, fs.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return FileUsage{}, err
	}
	return usage, nil
}

func usageCache(path string) (string, fs.FileInfo, error) {
	if path == "" {
		return "", nil, nil
	}
	resolved, err := filepath.EvalSymlinks(path)
	if errors.Is(err, fs.ErrNotExist) {
		return filepath.Clean(path), nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	info, err := os.Stat(resolved)
	if errors.Is(err, fs.ErrNotExist) {
		err = nil
	}
	return resolved, info, err
}

func countDirectoryFiles(ctx context.Context, root *os.Root, expected fs.FileInfo, cache string, cacheInfo fs.FileInfo, usage *FileUsage) error {
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	actual, err := dir.Stat()
	if err != nil {
		return err
	}
	if actual.Mode().Type() != fs.ModeDir || !os.SameFile(expected, actual) {
		return fs.ErrInvalid
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, readErr := dir.ReadDir(128)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return readErr
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			name := entry.Name()
			if cache != "" {
				if rel, err := filepath.Rel(cache, filepath.Join(root.Name(), name)); err == nil && rel == "." {
					continue
				}
			}
			info, err := root.Lstat(name)
			if err != nil {
				return err
			}
			switch info.Mode().Type() {
			case fs.ModeDir:
				if cacheInfo != nil && os.SameFile(info, cacheInfo) {
					continue
				}
				child, err := root.OpenRoot(name)
				if err != nil {
					return err
				}
				err = countDirectoryFiles(ctx, child, info, cache, cacheInfo, usage)
				child.Close()
				if err != nil {
					return err
				}
			case 0:
				if info.Size() < 0 || uint64(info.Size()) > math.MaxUint64-usage.Bytes {
					return fs.ErrInvalid
				}
				usage.Files++
				usage.Bytes += uint64(info.Size())
			}
		}
		if readErr != nil {
			return nil
		}
	}
}

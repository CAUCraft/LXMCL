package log

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

const RecentLimit = 30

type Entry struct {
	Path     string
	Modified time.Time
	info     fs.FileInfo
}

// Minecraft rotates dated logs; mod loaders also rotate numbered debug logs.
var archivedLogName = regexp.MustCompile(`^(?:[0-9]{4}-[0-9]{2}-[0-9]{2}-[0-9]+|debug-[0-9]+)\.log\.gz$`)

// IsPreview recognizes launcher, game and crash diagnostics in their own directories.
func IsPreview(path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	name, dir := filepath.Base(path), filepath.Base(filepath.Dir(path))
	return IsGame(path) || dir == "logs" && (name == "latest.log" || name == "debug.log" || archivedLogName.MatchString(name)) ||
		dir == "crash-reports" && strings.HasPrefix(name, "crash-") && strings.HasSuffix(name, ".txt")
}

// Recent scans only diagnostic directories and keeps a bounded, newest-first list.
// Missing directories are normal for a new instance. Other failures are returned
// alongside any entries found in readable directories.
func Recent(ctx context.Context, gameDir string) ([]Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(gameDir) {
		return nil, i18n.Error("instance.game_dir_required")
	}
	root, err := os.OpenRoot(gameDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	var entries []Entry
	var failures []error
	for _, directory := range []string{GameDirectory, "logs", "crash-reports"} {
		if err := ctx.Err(); err != nil {
			return entries, errors.Join(append(failures, err)...)
		}
		err := func() error {
			dir, err := root.Open(directory)
			if err != nil {
				return err
			}
			defer dir.Close()
			for {
				if err := ctx.Err(); err != nil {
					return err
				}
				files, err := dir.ReadDir(64)
				for _, file := range files {
					path := filepath.Join(gameDir, directory, file.Name())
					if !file.Type().IsRegular() || !IsPreview(path) {
						continue
					}
					info, infoErr := file.Info()
					if infoErr != nil {
						if errors.Is(infoErr, os.ErrNotExist) {
							continue
						}
						return infoErr
					}
					if !info.Mode().IsRegular() {
						continue
					}
					entries = append(entries, Entry{Path: path, Modified: info.ModTime()})
					slices.SortFunc(entries, func(a, b Entry) int {
						if n := b.Modified.Compare(a.Modified); n != 0 {
							return n
						}
						return strings.Compare(a.Path, b.Path)
					})
					entries = entries[:min(len(entries), RecentLimit)]
				}
				if err != nil {
					if errors.Is(err, io.EOF) {
						return nil
					}
					return err
				}
			}
		}()
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			failures = append(failures, i18n.Error("log.directory_read_failed", directory, err))
		}
	}
	// Capture stable identities only for the retained rows. On Windows,
	// directory enumeration may defer fetching file identities until comparison.
	for i := range entries {
		if err := ctx.Err(); err != nil {
			return entries, errors.Join(append(failures, err)...)
		}
		path := entries[i].Path
		info, err := root.Lstat(filepath.Join(filepath.Base(filepath.Dir(path)), filepath.Base(path)))
		if err == nil && info.Mode().IsRegular() {
			entries[i].info = info
		}
	}
	return entries, errors.Join(failures...)
}

// CheckReveal verifies that the listed original still occupies its instance path.
func (e Entry) CheckReveal(gameDir string) error {
	if e.info == nil || !filepath.IsAbs(gameDir) || !IsPreview(e.Path) {
		return fs.ErrInvalid
	}
	folder := filepath.Base(filepath.Dir(e.Path))
	name := filepath.Join(folder, filepath.Base(e.Path))
	relative, err := filepath.Rel(gameDir, e.Path)
	if err != nil || relative != name {
		return fs.ErrInvalid
	}
	root, err := os.OpenRoot(gameDir)
	if err != nil {
		return err
	}
	defer root.Close()
	parent, err := root.Lstat(folder)
	if err != nil {
		return err
	}
	if !parent.IsDir() {
		return fs.ErrInvalid
	}
	actual, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if !actual.Mode().IsRegular() || !os.SameFile(e.info, actual) {
		return fs.ErrInvalid
	}
	current, err := root.Lstat(folder)
	if err != nil {
		return err
	}
	if !current.IsDir() || !os.SameFile(parent, current) {
		return fs.ErrInvalid
	}
	return nil
}

package log

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

const RecentLimit = 30

type Entry struct {
	Path     string
	Modified time.Time
}

// IsPreview recognizes uncompressed launcher logs, latest.log and crash reports.
func IsPreview(path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	name, dir := filepath.Base(path), filepath.Base(filepath.Dir(path))
	return IsGame(path) || dir == "logs" && name == "latest.log" ||
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
	return entries, errors.Join(failures...)
}

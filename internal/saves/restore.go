package saves

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

// Restore imports ZIPs whose top-level folders contain level.dat.
// Existing worlds are kept; a conflicting name receives a new suffix.
func Restore(ctx context.Context, gameDir, source string) (int, error) {
	z, err := zip.OpenReader(source)
	if err != nil {
		return 0, err
	}
	defer z.Close()
	if len(z.File) == 0 || len(z.File) > maxSaveEntries {
		return 0, i18n.Error("saves.archive_entry_count_invalid")
	}
	worlds := make(map[string]bool)
	var size uint64
	for _, entry := range z.File {
		name := strings.TrimSuffix(entry.Name, "/")
		if !fs.ValidPath(name) || name == "." || !filepath.IsLocal(filepath.FromSlash(name)) ||
			strings.ContainsAny(name, ":\\") || (!entry.Mode().IsRegular() && !entry.FileInfo().IsDir()) {
			return 0, i18n.Error("saves.archive_path_or_type_invalid", entry.Name)
		}
		world, rest, nested := strings.Cut(name, "/")
		if !nested && !entry.FileInfo().IsDir() {
			return 0, i18n.Error("saves.archive_world_directory_required")
		}
		worlds[world] = worlds[world] || (rest == "level.dat" && entry.Mode().IsRegular())
		if entry.UncompressedSize64 > maxSaveBytes-size {
			return 0, i18n.Error("saves.archive_expanded_too_large")
		}
		size += entry.UncompressedSize64
	}
	names := make([]string, 0, len(worlds))
	for name, valid := range worlds {
		if !valid {
			return 0, i18n.Error("saves.archive_level_dat_missing", name)
		}
		names = append(names, name)
	}
	sort.Strings(names)
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
	for _, entry := range z.File {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		name := path.Join(stage, entry.Name)
		if entry.FileInfo().IsDir() {
			if err := root.MkdirAll(name, 0755); err != nil {
				return 0, err
			}
			continue
		}
		if path.Base(name) == "session.lock" {
			continue
		}
		if err := root.MkdirAll(path.Dir(name), 0755); err != nil {
			return 0, err
		}
		if err := extractSaveFile(ctx, root, name, entry); err != nil {
			return 0, err
		}
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	for n, world := range names {
		dst := world
		for suffix := 2; ; suffix++ {
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
	return len(names), nil
}

func extractSaveFile(ctx context.Context, root *os.Root, name string, entry *zip.File) error {
	src, err := entry.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	n, err := io.Copy(dst, io.LimitReader(transfer.Reader(ctx, src), int64(entry.UncompressedSize64)+1))
	if err == nil && uint64(n) != entry.UncompressedSize64 {
		err = i18n.Error("saves.archive_size_mismatch")
	}
	return errors.Join(err, dst.Close())
}

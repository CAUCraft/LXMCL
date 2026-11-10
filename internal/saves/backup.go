package saves

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

func Backup(ctx context.Context, gameDir, dataRoot string, id int) (string, error) {
	root, err := savesRoot(gameDir, false)
	if errors.Is(err, fs.ErrNotExist) {
		return "", i18n.Error("saves.no_worlds_to_back_up")
	}
	if err != nil {
		return "", err
	}
	defer root.Close()
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return "", err
	}
	var worlds []string
	for _, entry := range entries {
		if entry.IsDir() {
			if level, err := root.Lstat(filepath.Join(entry.Name(), "level.dat")); err == nil && level.Mode().IsRegular() {
				worlds = append(worlds, entry.Name())
			}
		}
	}
	if len(worlds) == 0 {
		return "", i18n.Error("saves.no_worlds_to_back_up")
	}
	dir := BackupsDir(dataRoot, id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, ".lxmcl-backup-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	z := zip.NewWriter(f)
	var total uint64
	files := 0
	for _, world := range worlds {
		err = fs.WalkDir(root.FS(), world, func(name string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.IsDir() || entry.Name() == "session.lock" {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return i18n.Error("saves.backup_file_unsupported", name)
			}
			files++
			if files > maxSaveEntries {
				return i18n.Error("saves.backup_too_many_files")
			}
			total += uint64(info.Size())
			if total > maxSaveBytes {
				return i18n.Error("saves.backup_too_large")
			}
			header, err := zip.FileInfoHeader(info)
			if err != nil {
				return err
			}
			header.Name, header.Method = name, zip.Deflate
			dst, err := z.CreateHeader(header)
			if err != nil {
				return err
			}
			src, err := root.Open(filepath.FromSlash(name))
			if err != nil {
				return err
			}
			defer src.Close()
			n, err := io.Copy(dst, io.LimitReader(transfer.Reader(ctx, src), info.Size()+1))
			if err == nil && n != info.Size() {
				return i18n.Error("saves.backup_file_changed", name)
			}
			return err
		})
		if err != nil {
			break
		}
	}
	if err := errors.Join(err, z.Close(), f.Close(), ctx.Err()); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, time.Now().Format("20060102-150405")+"-"+rand.Text()[:6]+".zip")
	return dst, os.Rename(f.Name(), dst)
}

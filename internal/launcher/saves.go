package launcher

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
	"time"
)

const (
	maxSaveBytes   = 32 << 30
	maxSaveEntries = 50000
)

func savesRoot(i Instance, create bool) (*os.Root, error) {
	if !filepath.IsAbs(i.GameDir) {
		return nil, errors.New("实例需要绝对游戏目录")
	}
	if create {
		if err := os.MkdirAll(i.GameDir, 0755); err != nil {
			return nil, err
		}
	}
	root, err := os.OpenRoot(i.GameDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if create {
		if err := root.Mkdir("saves", 0755); err != nil && !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
	}
	return root.OpenRoot("saves")
}

func BackupsDir(root string, id int) string {
	return filepath.Join(root, "backups", fmt.Sprintf("instance-%d", id))
}

func BackupSaves(ctx context.Context, i Instance, dataRoot string) (string, error) {
	root, err := savesRoot(i, false)
	if errors.Is(err, fs.ErrNotExist) {
		return "", errors.New("此实例还没有可备份的存档")
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
		return "", errors.New("此实例还没有可备份的存档")
	}
	dir := BackupsDir(dataRoot, i.ID)
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
				return fmt.Errorf("存档包含不支持的文件：%s", name)
			}
			files++
			if files > maxSaveEntries {
				return errors.New("存档备份超过 50000 个文件")
			}
			total += uint64(info.Size())
			if total > maxSaveBytes {
				return errors.New("存档备份超过 32 GiB")
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
			n, err := io.Copy(dst, io.LimitReader(cancelReader{ctx: ctx, r: src}, info.Size()+1))
			if err == nil && n != info.Size() {
				return fmt.Errorf("备份时存档发生变动：%s", name)
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

// RestoreSaves imports ZIPs whose top-level folders contain level.dat.
// Existing worlds are kept; a conflicting name receives a new suffix.
func RestoreSaves(ctx context.Context, i Instance, source string) (int, error) {
	z, err := zip.OpenReader(source)
	if err != nil {
		return 0, err
	}
	defer z.Close()
	if len(z.File) == 0 || len(z.File) > maxSaveEntries {
		return 0, errors.New("存档归档条目数量不受支持")
	}
	worlds := make(map[string]bool)
	var size uint64
	for _, entry := range z.File {
		name := strings.TrimSuffix(entry.Name, "/")
		if !fs.ValidPath(name) || name == "." || !filepath.IsLocal(filepath.FromSlash(name)) ||
			strings.ContainsAny(name, ":\\") || (!entry.Mode().IsRegular() && !entry.FileInfo().IsDir()) {
			return 0, fmt.Errorf("存档归档路径或文件类型不安全：%s", entry.Name)
		}
		world, rest, nested := strings.Cut(name, "/")
		if !nested && !entry.FileInfo().IsDir() {
			return 0, errors.New("ZIP 中应包含带 level.dat 的世界文件夹")
		}
		worlds[world] = worlds[world] || (rest == "level.dat" && entry.Mode().IsRegular())
		if entry.UncompressedSize64 > maxSaveBytes-size {
			return 0, errors.New("解压后存档超过 32 GiB")
		}
		size += entry.UncompressedSize64
	}
	names := make([]string, 0, len(worlds))
	for name, valid := range worlds {
		if !valid {
			return 0, fmt.Errorf("存档目录 %s 缺少 level.dat", name)
		}
		names = append(names, name)
	}
	sort.Strings(names)
	root, err := savesRoot(i, true)
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
			return n, fmt.Errorf("已恢复 %d 个存档：%w", n, err)
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
	n, err := io.Copy(dst, io.LimitReader(cancelReader{ctx: ctx, r: src}, int64(entry.UncompressedSize64)+1))
	if err == nil && uint64(n) != entry.UncompressedSize64 {
		err = errors.New("存档归档大小不符")
	}
	return errors.Join(err, dst.Close())
}

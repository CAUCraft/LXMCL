package modpack

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/distribution"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

type Selection struct {
	Mods, Config, Worlds    bool
	Screenshots, ServerList bool // Local instance copies only.
}

func (s Selection) walk(ctx context.Context, dir string, write func(string, io.Reader) error) error {
	if !filepath.IsAbs(dir) {
		return i18n.Error("instance.game_dir_required")
	}
	root, err := os.OpenRoot(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	var folders []string
	if s.Mods {
		folders = append(folders, "mods", "resourcepacks", "shaderpacks")
	}
	if s.Config {
		folders = append(folders, "options.txt", "config", "defaultconfigs", "kubejs", "scripts")
	}
	if s.Worlds {
		folders = append(folders, "saves")
	}
	var count int
	var total int64
	for _, folder := range folders {
		entry, err := root.Lstat(folder)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if entry.Mode()&os.ModeSymlink != 0 {
			return i18n.Error("modpack.archive_path_invalid", folder)
		}
		err = fs.WalkDir(root.FS(), folder, func(name string, e fs.DirEntry, err error) error {
			if name == folder && errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			if err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if !storage.ValidPortablePath(name) {
				return i18n.Error("modpack.archive_path_invalid", name)
			}
			if e.IsDir() {
				return nil
			}
			if !e.Type().IsRegular() {
				return i18n.Error("modpack.archive_path_invalid", name)
			}
			info, err := e.Info()
			if err != nil {
				return err
			}
			count++
			total += info.Size()
			if count >= maxEntries || info.Size() > distribution.MaxFileSize || total >= maxBytes {
				return i18n.Error("modpack.archive_expanded_too_large")
			}
			file, err := root.Open(name)
			if err != nil {
				return err
			}
			defer file.Close()
			limited := &io.LimitedReader{R: transfer.Reader(ctx, file), N: info.Size() + 1}
			if err := write(name, limited); err != nil {
				return err
			}
			if limited.N != 1 {
				return i18n.Error("modpack.archive_size_changed")
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return ctx.Err()
}

func separateFolder(source, dest string) bool {
	if !filepath.IsAbs(source) || !filepath.IsAbs(dest) {
		return false
	}
	if real, err := filepath.EvalSymlinks(source); err == nil {
		source = real
	}
	if real, err := filepath.EvalSymlinks(dest); err == nil {
		dest = real
	}
	rel, err := filepath.Rel(source, dest)
	return err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func Duplicate(ctx context.Context, src, dst instance.Instance, selection Selection, commit func() error) error {
	parent := filepath.Dir(dst.GameDir)
	if !separateFolder(src.GameDir, parent) {
		return i18n.Error("modpack.instance_directory_exists")
	}
	if err := os.MkdirAll(parent, 0755); err != nil {
		return err
	}
	temp, err := os.MkdirTemp(parent, ".lxmcl-copy-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	root, err := os.OpenRoot(temp)
	if err != nil {
		return err
	}
	err = selection.walk(ctx, src.GameDir, func(name string, reader io.Reader) error {
		if err := root.MkdirAll(path.Dir(name), 0755); err != nil {
			return err
		}
		f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			return err
		}
		_, err = io.Copy(f, reader)
		return errors.Join(err, f.Close())
	})
	if err == nil && selection.ServerList {
		err = content.CopyServerList(ctx, src.GameDir, root)
	}
	if err == nil && selection.Screenshots {
		err = content.CopyScreenshots(ctx, src.GameDir, root)
	}
	if err = errors.Join(err, root.Close()); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := os.Lstat(dst.GameDir); !errors.Is(err, fs.ErrNotExist) {
		if err != nil {
			return err
		}
		return i18n.Error("modpack.instance_directory_exists")
	}
	if err := os.Rename(temp, dst.GameDir); err != nil {
		return err
	}
	if err := commit(); err != nil {
		return errors.Join(err, os.Rename(dst.GameDir, temp))
	}
	return nil
}

func Export(ctx context.Context, i instance.Instance, folder, version string, selection Selection) (string, error) {
	if err := i.Validate(); err != nil {
		return "", err
	}
	if strings.TrimSpace(version) == "" || !separateFolder(i.GameDir, folder) {
		return "", i18n.Error("modpack.export_destination_invalid")
	}
	root, err := os.OpenRoot(folder)
	if err != nil {
		return "", err
	}
	defer root.Close()
	name := fmt.Sprintf("instance-%d-%s.mrpack", i.ID, version)
	deps := map[string]string{"minecraft": i.Version}
	if !i.Vanilla() {
		key := strings.ToLower(i.Loader)
		if i.UsesKnot() {
			key += "-loader"
		}
		deps[key] = i.LoaderVersion
	}
	manifest := map[string]any{"formatVersion": 1, "game": "minecraft", "versionId": version, "name": i.Name, "dependencies": deps, "files": []any{}}
	err = storage.PublishNew(ctx, root, name, 0644, func(f *os.File) error {
		z := zip.NewWriter(f)
		index, err := z.Create("modrinth.index.json")
		if err != nil {
			return err
		}
		if err := json.NewEncoder(index).Encode(manifest); err != nil {
			return err
		}
		err = selection.walk(ctx, i.GameDir, func(name string, r io.Reader) error {
			w, err := z.Create("overrides/" + name)
			if err != nil {
				return err
			}
			_, err = io.Copy(w, r)
			return err
		})
		if err != nil {
			return err
		}
		if err := z.Close(); err != nil {
			return err
		}
		st, err := f.Stat()
		if err != nil {
			return err
		}
		if st.Size() > MaxArchiveSize {
			return i18n.Error("modpack.archive_expanded_too_large")
		}
		_, err = readPlan(f, st.Size(), Modrinth)
		return err
	})
	if errors.Is(err, fs.ErrExist) {
		err = i18n.Error("modpack.export_name_conflict", err)
	}
	return filepath.Join(folder, name), err
}

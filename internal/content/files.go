package content

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

func OpenDirectory(i instance.Instance, kind Kind, create bool) (*os.Root, error) {
	if _, _, err := kind.Format(i); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(i.GameDir) {
		return nil, i18n.Error("instance.game_dir_required")
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
		if err := root.Mkdir(string(kind), 0755); err != nil && !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
	}
	return root.OpenRoot(string(kind))
}

func List(i instance.Instance, kind Kind) ([]File, error) {
	ext, _, err := kind.Format(i)
	if err != nil {
		return nil, err
	}
	root, err := OpenDirectory(i, kind, false)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return nil, err
	}
	var result []File
	for _, entry := range entries {
		name, disabled := strings.CutSuffix(entry.Name(), ".disabled")
		if !entry.Type().IsRegular() || !strings.EqualFold(filepath.Ext(name), ext) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		result = append(result, File{Name: name, Size: info.Size(), Enabled: !disabled})
	}
	return result, nil
}

func ValidName(name, ext string) bool {
	return filepath.IsLocal(name) && filepath.Base(name) == name &&
		!strings.ContainsAny(name, "/\\:<>|\"?*") && strings.EqualFold(filepath.Ext(name), ext)
}

func SetEnabled(i instance.Instance, kind Kind, name string, enabled bool) error {
	ext, _, err := kind.Format(i)
	if err != nil {
		return err
	}
	if !ValidName(name, ext) {
		return i18n.Error("resource.filename_invalid")
	}
	root, err := OpenDirectory(i, kind, false)
	if err != nil {
		return err
	}
	defer root.Close()
	from, to := name, name+".disabled"
	if enabled {
		from, to = to, from
	}
	info, err := root.Lstat(from)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return i18n.Error("resource.not_regular")
	}
	if _, err := root.Lstat(to); !errors.Is(err, fs.ErrNotExist) {
		if err != nil {
			return err
		}
		return i18n.Error("resource.name_conflict")
	}
	return root.Rename(from, to)
}

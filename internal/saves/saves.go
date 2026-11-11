package saves

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

const (
	maxSaveBytes   = 32 << 30
	maxSaveEntries = 50000
)

func savesRoot(gameDir string, create bool) (*os.Root, error) {
	if !filepath.IsAbs(gameDir) {
		return nil, i18n.Error("instance.game_dir_required")
	}
	if create {
		if err := os.MkdirAll(gameDir, 0755); err != nil {
			return nil, err
		}
	}
	root, err := os.OpenRoot(gameDir)
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

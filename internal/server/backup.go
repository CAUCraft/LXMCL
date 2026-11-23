package server

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

const favoritesFormat = "lxmcl-server-favorites/v1"
const maxFavoritesBackup = 8 << 20

type favoritesBackup struct {
	Format  string   `json:"format"`
	Servers []Server `json:"servers"`
}

func validateBackup(rows []Server) error {
	for n, row := range rows {
		if _, err := NewFavorite(row.Name, row.Address); err != nil {
			return i18n.Error("server.backup_entry_invalid", n+1, err)
		}
	}
	return nil
}

// ExportFavorites publishes a new portable file in the supplied order.
func ExportFavorites(ctx context.Context, path string, rows []Server) error {
	if err := validateBackup(rows); err != nil {
		return err
	}
	if rows == nil {
		rows = []Server{}
	}
	data, err := json.MarshalIndent(favoritesBackup{Format: favoritesFormat, Servers: rows}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if len(data) > maxFavoritesBackup {
		return i18n.Error("server.backup_large")
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer root.Close()
	return storage.PublishNew(ctx, root, filepath.Base(path), 0600, func(file *os.File) error {
		_, err := file.Write(data)
		return err
	})
}

// ReadFavorites validates the complete snapshot before making it available for preview.
func ReadFavorites(path string) ([]Server, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, i18n.Error("server.backup_invalid")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxFavoritesBackup+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxFavoritesBackup {
		return nil, i18n.Error("server.backup_large")
	}
	var b favoritesBackup
	if err := json.Unmarshal(data, &b); err != nil || b.Format != favoritesFormat || b.Servers == nil {
		return nil, i18n.Error("server.backup_invalid")
	}
	if err := validateBackup(b.Servers); err != nil {
		return nil, err
	}
	return b.Servers, nil
}

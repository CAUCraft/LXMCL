package server

import (
	"context"
	"encoding/binary"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/nbt"
)

// ExportGameList publishes a new standalone Java Edition server list.
func ExportGameList(ctx context.Context, path string, rows []Server) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !filepath.IsAbs(path) {
		return i18n.Error("server.game_export_path")
	}
	if len(rows) == 0 {
		return i18n.Error("server.game_export_empty")
	}
	if len(rows) > maxGameServers {
		return i18n.Error("server.game_list_large")
	}
	// Named root compound with an empty name and a list of server compounds.
	data := []byte{10, 0, 0, 9, 0, 7, 's', 'e', 'r', 'v', 'e', 'r', 's', 10}
	data = binary.BigEndian.AppendUint32(data, uint32(len(rows)))
	for i, row := range rows {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := NewFavorite(row.Name, row.Address); err != nil {
			return i18n.Error("server.game_export_entry", i+1, err)
		}
		var err error
		data, err = nbt.AppendJavaString(append(data, 8, 0, 4, 'n', 'a', 'm', 'e'), row.Name)
		if err != nil {
			return i18n.Error("server.game_export_entry", i+1, err)
		}
		data, err = nbt.AppendJavaString(append(data, 8, 0, 2, 'i', 'p'), row.Address)
		if err != nil {
			return i18n.Error("server.game_export_entry", i+1, err)
		}
		data = append(data, 0) // End of the server compound.
		if len(data) >= nbt.MaxBytes {
			return i18n.Error("server.game_list_large")
		}
	}
	return publishServerFile(ctx, path, append(data, 0))
}

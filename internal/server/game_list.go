package server

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/nbt"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

const maxGameServers = 65536

// ReadGameList reads the visible favorites of a Java Edition instance, in file order.
// An absent game directory or servers.dat is reported as os.ErrNotExist.
func ReadGameList(ctx context.Context, gameDir string) ([]Server, error) {
	if !filepath.IsAbs(gameDir) {
		return nil, i18n.Error("instance.game_dir_required")
	}
	root, err := os.OpenRoot(gameDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	info, err := root.Lstat("servers.dat")
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, i18n.Error("server.game_list_invalid")
	}
	if info.Size() > nbt.MaxBytes {
		return nil, i18n.Error("server.game_list_large")
	}
	file, err := root.Open("servers.dat")
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(transfer.Reader(ctx, file), nbt.MaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > nbt.MaxBytes {
		return nil, i18n.Error("server.game_list_large")
	}
	return decodeGameList(ctx, data)
}

// servers.dat uses an uncompressed, named, big-endian NBT compound.
func decodeGameList(ctx context.Context, data []byte) ([]Server, error) {
	var entries []Server
	found, oversized := false, false
	err := nbt.Read(ctx, data, func(r *nbt.Reader, kind byte, key string) {
		if key != "servers" {
			r.Skip(kind)
			return
		}
		if kind != 9 || found {
			r.Invalidate()
			return
		}
		found = true
		element, count := r.Byte(), r.Count()
		if count > 0 && element != 10 {
			r.Invalidate()
			return
		}
		if count > maxGameServers {
			oversized = true
			r.Invalidate()
			return
		}
		for range count {
			if r.Err() != nil {
				break
			}
			var s Server
			hidden := false
			r.Compound(func(tag byte, name string) {
				switch {
				case tag == 8 && name == "name":
					s.Name, _ = r.JavaString()
				case tag == 8 && name == "ip":
					s.Address, _ = r.JavaString()
				case tag == 1 && name == "hidden":
					hidden = r.Byte() != 0
				default:
					r.Skip(tag)
				}
			})
			if !hidden && r.Err() == nil {
				entries = append(entries, s)
			}
		}
	})
	if oversized {
		return nil, i18n.Error("server.game_list_large")
	}
	if err != nil && !errors.Is(err, nbt.ErrInvalid) {
		return nil, err
	}
	if !found || err != nil {
		return nil, i18n.Error("server.game_list_invalid")
	}
	return entries, nil
}

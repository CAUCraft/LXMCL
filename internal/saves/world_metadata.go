package saves

import (
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/nbt"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

// LastPlayed is expressed in Unix milliseconds. Keep the year within the
// four-digit range displayed by the launcher.
const maxLastPlayedMillis = 253402300799999

func readWorldMetadata(ctx context.Context, root *os.Root, world World) (string, time.Time, string) {
	var name, version string
	var lastPlayed time.Time
	err := readWorldData(ctx, root, world, func(r *nbt.Reader, tag byte, key string) {
		switch {
		case tag == 8 && key == "LevelName":
			if value := worldMetadataString(r); value != "" {
				name = value
			}
		case tag == 4 && key == "LastPlayed":
			ms := r.Long()
			if ms > 0 && ms <= maxLastPlayedMillis {
				lastPlayed = time.UnixMilli(ms).UTC()
			}
		case key == "Version":
			version = ""
			if tag != 10 {
				r.Skip(tag)
				return
			}
			r.Compound(func(tag byte, key string) {
				if key == "Name" {
					version = ""
					if tag == 8 {
						version = worldMetadataString(r)
						return
					}
				}
				r.Skip(tag)
			})
		default:
			r.Skip(tag)
		}
	})
	if err != nil {
		return "", time.Time{}, ""
	}
	return name, lastPlayed, version
}

func worldMetadataString(r *nbt.Reader) string {
	value, valid := r.JavaString()
	if !valid || strings.TrimSpace(value) == "" || strings.ContainsFunc(value, unicode.IsControl) {
		return ""
	}
	return value
}

// readWorldData reads the saved Data compound from the captured level.dat.
func readWorldData(ctx context.Context, root *os.Root, world World, visit func(*nbt.Reader, byte, string)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if world.level.Size() > nbt.MaxBytes {
		return nbt.ErrInvalid
	}
	file, err := root.Open(filepath.Join(world.folder, "level.dat"))
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !sameBackupFile(world.level, info) {
		return i18n.Error("saves.world_changed", world.folder)
	}
	zipped := io.LimitReader(file, nbt.MaxBytes+1)
	stream, err := gzip.NewReader(transfer.Reader(ctx, zipped))
	if err != nil {
		return err
	}
	defer stream.Close()
	data, err := io.ReadAll(io.LimitReader(transfer.Reader(ctx, stream), nbt.MaxBytes+1))
	if err != nil {
		return err
	}
	info, err = file.Stat()
	if err != nil {
		return err
	}
	if !sameBackupFile(world.level, info) {
		return i18n.Error("saves.world_changed", world.folder)
	}
	found := false
	return nbt.Read(ctx, data, func(r *nbt.Reader, tag byte, key string) {
		if key != "Data" {
			r.Skip(tag)
			return
		}
		if tag != 10 || found {
			r.Invalidate()
			return
		}
		found = true
		r.Compound(func(tag byte, key string) { visit(r, tag, key) })
	})
}

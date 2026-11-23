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

	"github.com/CAUCraft/LXMCL/internal/nbt"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

// LastPlayed is expressed in Unix milliseconds. Keep the year within the
// four-digit range displayed by the launcher.
const maxLastPlayedMillis = 253402300799999

func readWorldMetadata(ctx context.Context, root *os.Root, world World) (string, time.Time) {
	if world.level.Size() > nbt.MaxBytes {
		return "", time.Time{}
	}
	file, err := root.Open(filepath.Join(world.folder, "level.dat"))
	if err != nil {
		return "", time.Time{}
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !os.SameFile(world.level, info) || world.level.Size() != info.Size() || !world.level.ModTime().Equal(info.ModTime()) {
		return "", time.Time{}
	}
	zipped := io.LimitReader(file, nbt.MaxBytes+1)
	stream, err := gzip.NewReader(transfer.Reader(ctx, zipped))
	if err != nil {
		return "", time.Time{}
	}
	defer stream.Close()
	data, err := io.ReadAll(io.LimitReader(transfer.Reader(ctx, stream), nbt.MaxBytes+1))
	if err != nil {
		return "", time.Time{}
	}
	var name string
	var lastPlayed time.Time
	err = nbt.Read(ctx, data, func(r *nbt.Reader, tag byte, key string) {
		if tag != 10 || key != "Data" {
			r.Skip(tag)
			return
		}
		r.Compound(func(tag byte, key string) {
			switch {
			case tag == 8 && key == "LevelName":
				value, valid := r.JavaString()
				if valid && strings.TrimSpace(value) != "" && !strings.ContainsFunc(value, unicode.IsControl) {
					name = value
				}
			case tag == 4 && key == "LastPlayed":
				ms := r.Long()
				if ms > 0 && ms <= maxLastPlayedMillis {
					lastPlayed = time.UnixMilli(ms).UTC()
				}
			default:
				r.Skip(tag)
			}
		})
	})
	if err != nil {
		return "", time.Time{}
	}
	return name, lastPlayed
}

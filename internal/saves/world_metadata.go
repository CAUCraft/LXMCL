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

// GameMode represents a world's recorded default mode.
type GameMode uint8

const (
	GameModeUnknown GameMode = iota
	GameModeSurvival
	GameModeCreative
	GameModeAdventure
	GameModeSpectator
)

type worldHardcore uint8

const (
	hardcoreUnknown worldHardcore = iota
	hardcoreOff
	hardcoreOn
)

type worldMetadata struct {
	name, savedVersion string
	lastPlayed         time.Time
	gameMode           GameMode
	hardcore           worldHardcore
}

func readWorldMetadata(ctx context.Context, root *os.Root, world World) worldMetadata {
	var metadata worldMetadata
	var modernHardcore worldHardcore
	var hasDifficultySettings bool
	err := readWorldData(ctx, root, world, func(r *nbt.Reader, tag byte, key string) {
		switch {
		case tag == 8 && key == "LevelName":
			if value := worldMetadataString(r); value != "" {
				metadata.name = value
			}
		case tag == 4 && key == "LastPlayed":
			ms := r.Long()
			if ms > 0 && ms <= maxLastPlayedMillis {
				metadata.lastPlayed = time.UnixMilli(ms).UTC()
			}
		case key == "GameType":
			metadata.gameMode = GameModeUnknown
			if tag != 3 {
				r.Skip(tag)
				return
			}
			// Java's stored IDs run from Survival (0) through Spectator (3).
			if mode := r.Int(); mode >= 0 && mode <= 3 {
				metadata.gameMode = GameModeSurvival + GameMode(mode)
			}
		case key == "hardcore":
			metadata.hardcore = readWorldHardcore(r, tag)
		case key == "difficulty_settings":
			// Java 26.1 moved the flag here. A present group takes precedence.
			hasDifficultySettings, modernHardcore = true, hardcoreUnknown
			if tag != 10 {
				r.Skip(tag)
				return
			}
			r.Compound(func(tag byte, key string) {
				if key == "hardcore" {
					modernHardcore = readWorldHardcore(r, tag)
				} else {
					r.Skip(tag)
				}
			})
		case key == "Version":
			metadata.savedVersion = ""
			if tag != 10 {
				r.Skip(tag)
				return
			}
			r.Compound(func(tag byte, key string) {
				if key == "Name" {
					metadata.savedVersion = ""
					if tag == 8 {
						metadata.savedVersion = worldMetadataString(r)
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
		return worldMetadata{}
	}
	if hasDifficultySettings {
		metadata.hardcore = modernHardcore
	}
	return metadata
}

func readWorldHardcore(r *nbt.Reader, tag byte) worldHardcore {
	if tag != 1 {
		r.Skip(tag)
		return hardcoreUnknown
	}
	switch r.Byte() {
	case 0:
		return hardcoreOff
	case 1:
		return hardcoreOn
	default:
		return hardcoreUnknown
	}
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

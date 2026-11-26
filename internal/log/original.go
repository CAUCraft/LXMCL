package log

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

// ErrOriginalChanged indicates that the selected file cannot be identified or read.
var ErrOriginalChanged = errors.New("original log changed or unavailable")

// ExportOriginal copies the selected catalog entry without decoding its contents.
// For a live log, the copy ends at the size observed after save confirmation.
func (e Entry) ExportOriginal(ctx context.Context, gameDir, destination string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !filepath.IsAbs(destination) {
		return i18n.Error("log.export_path_required")
	}
	ext := filepath.Ext(e.Path)
	name := filepath.Base(destination)
	if !strings.EqualFold(filepath.Ext(name), ext) {
		return i18n.Error("log.export_original_suffix", ext)
	}
	sourceName, err := e.originalName(gameDir)
	if err != nil {
		return originalSourceError(err)
	}
	gameDir = filepath.Clean(gameDir)
	game, err := os.OpenRoot(gameDir)
	if err != nil {
		return originalSourceError(err)
	}
	defer game.Close()
	gameInfo, err := game.Stat(".")
	if err != nil {
		return originalSourceError(err)
	}
	parent, err := e.checkOriginal(game, sourceName)
	if err != nil {
		return originalSourceError(err)
	}
	in, err := game.Open(sourceName)
	if err != nil {
		return originalSourceError(err)
	}
	defer in.Close()
	before, err := in.Stat()
	if err != nil {
		return originalSourceError(err)
	}
	if !before.Mode().IsRegular() || !os.SameFile(e.info, before) {
		return ErrOriginalChanged
	}

	dst, err := os.OpenRoot(filepath.Dir(filepath.Clean(destination)))
	if err != nil {
		return err
	}
	defer dst.Close()
	return storage.PublishNew(ctx, dst, name, 0600, func(out *os.File) error {
		if _, err := io.CopyN(out, transfer.Reader(ctx, in), before.Size()); err != nil {
			if errors.Is(err, io.EOF) {
				return ErrOriginalChanged
			}
			return err
		}
		after, err := in.Stat()
		if err != nil {
			return originalSourceError(err)
		}
		if !(after.Size() == before.Size() && after.ModTime().Equal(before.ModTime()) ||
			ext == ".log" && after.Size() > before.Size()) {
			return ErrOriginalChanged
		}
		current, err := e.checkOriginal(game, sourceName)
		if err != nil {
			return originalSourceError(err)
		}
		if !os.SameFile(parent, current) {
			return ErrOriginalChanged
		}
		currentGame, err := os.Stat(gameDir)
		if err != nil {
			return originalSourceError(err)
		}
		if !os.SameFile(gameInfo, currentGame) {
			return ErrOriginalChanged
		}
		return nil
	})
}

func originalSourceError(err error) error {
	return fmt.Errorf("%w: %w", ErrOriginalChanged, err)
}

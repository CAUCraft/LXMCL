// Package screenshots reads the PNG screenshots kept by Minecraft in an instance.
package screenshots

import (
	"cmp"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/transfer"
	"golang.org/x/image/draw"
)

const Directory = "screenshots"

type Entry struct {
	Name     string
	Modified time.Time
}

func open(gameDir string) (*os.Root, error) {
	if !filepath.IsAbs(gameDir) {
		return nil, i18n.Error("instance.game_dir_required")
	}
	game, err := os.OpenRoot(gameDir)
	if err != nil {
		return nil, err
	}
	defer game.Close()
	return game.OpenRoot(Directory)
}

// List returns the most recently modified screenshots first.
func List(ctx context.Context, gameDir string) ([]Entry, error) {
	root, err := open(gameDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	files, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return nil, err
	}
	var entries []Entry
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !f.Type().IsRegular() || !strings.EqualFold(filepath.Ext(f.Name()), ".png") {
			continue
		}
		info, err := f.Info()
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		entries = append(entries, Entry{f.Name(), info.ModTime()})
	}
	slices.SortFunc(entries, func(a, b Entry) int {
		if n := b.Modified.Compare(a.Modified); n != 0 {
			return n
		}
		return cmp.Compare(a.Name, b.Name)
	})
	return entries, nil
}

// Preview keeps at most a 1600-pixel image in the gallery and reports the original dimensions.
func Preview(ctx context.Context, gameDir, name string) (image.Image, image.Point, error) {
	var size image.Point
	if !filepath.IsLocal(name) || filepath.Base(name) != name || !strings.EqualFold(filepath.Ext(name), ".png") {
		return nil, size, fs.ErrInvalid
	}
	root, err := open(gameDir)
	if err != nil {
		return nil, size, err
	}
	defer root.Close()
	f, err := root.Open(name)
	if err != nil {
		return nil, size, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, size, err
	}
	if !info.Mode().IsRegular() {
		return nil, size, fs.ErrInvalid
	}
	const maxBytes = 128 << 20
	if info.Size() > maxBytes {
		return nil, size, i18n.Error("screenshot.too_large")
	}
	cfg, err := png.DecodeConfig(transfer.Reader(ctx, f))
	if err != nil {
		return nil, size, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > (36<<20)/cfg.Height {
		return nil, size, i18n.Error("screenshot.too_large")
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return nil, size, err
	}
	src, err := png.Decode(transfer.Reader(ctx, io.LimitReader(f, maxBytes)))
	if err != nil {
		return nil, size, err
	}
	size = image.Pt(cfg.Width, cfg.Height)
	const side = 1600
	if den := max(size.X, size.Y); den > side {
		w, h := max(1, size.X*side/den), max(1, size.Y*side/den)
		scaled := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.ApproxBiLinear.Scale(scaled, scaled.Bounds(), src, src.Bounds(), draw.Src, nil)
		return scaled, size, ctx.Err()
	}
	return src, size, ctx.Err()
}

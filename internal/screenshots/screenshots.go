// Package screenshots reads the PNG screenshots kept by Minecraft in an instance.
package screenshots

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
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
	info     fs.FileInfo
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
		info, err := root.Lstat(f.Name())
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if info.Mode().IsRegular() {
			entries = append(entries, Entry{Name: f.Name(), Modified: info.ModTime(), info: info})
		}
	}
	slices.SortFunc(entries, func(a, b Entry) int {
		if n := b.Modified.Compare(a.Modified); n != 0 {
			return n
		}
		return cmp.Compare(a.Name, b.Name)
	})
	return entries, nil
}

// Original identifies the full-resolution PNG that produced a gallery preview.
type Original struct {
	Size          image.Point
	gameDir, name string
	digest        [sha256.Size]byte
}

func readPNG(ctx context.Context, gameDir, name string) ([]byte, error) {
	if !filepath.IsLocal(name) || filepath.Base(name) != name || !strings.EqualFold(filepath.Ext(name), ".png") {
		return nil, fs.ErrInvalid
	}
	root, err := open(gameDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fs.ErrInvalid
	}
	const maxBytes = 128 << 20
	if info.Size() > maxBytes {
		return nil, i18n.Error("screenshot.too_large")
	}
	data, err := io.ReadAll(transfer.Reader(ctx, io.LimitReader(f, info.Size()+1)))
	if err == nil && int64(len(data)) != info.Size() {
		return nil, i18n.Error("screenshot.changed")
	}
	return data, err
}

// PNG rereads the selected original without retaining full-size images while browsing.
func (o *Original) PNG(ctx context.Context) ([]byte, error) {
	data, err := readPNG(ctx, o.gameDir, o.name)
	if err != nil {
		return nil, err
	}
	if sha256.Sum256(data) != o.digest {
		return nil, i18n.Error("screenshot.changed")
	}
	return data, ctx.Err()
}

// Preview limits the displayed image to 1600 pixels and binds it to its original.
func Preview(ctx context.Context, gameDir, name string) (image.Image, *Original, error) {
	data, err := readPNG(ctx, gameDir, name)
	if err != nil {
		return nil, nil, err
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > (36<<20)/cfg.Height {
		return nil, nil, i18n.Error("screenshot.too_large")
	}
	img, err := png.Decode(transfer.Reader(ctx, bytes.NewReader(data)))
	if err != nil {
		return nil, nil, err
	}
	size := img.Bounds().Size()
	const side = 1600
	if den := max(size.X, size.Y); den > side {
		w, h := max(1, size.X*side/den), max(1, size.Y*side/den)
		scaled := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.ApproxBiLinear.Scale(scaled, scaled.Bounds(), img, img.Bounds(), draw.Src, nil)
		img = scaled
	}
	return img, &Original{Size: size, gameDir: gameDir, name: name, digest: sha256.Sum256(data)}, ctx.Err()
}

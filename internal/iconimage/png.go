// Package iconimage prepares small square images for launcher icons.
package iconimage

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/png"
	"io"
	"os"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/transfer"
	"golang.org/x/image/draw"
)

const side = 96
const MaxImportBytes = 32 << 20

// Fit preserves the aspect ratio and centers the image on a transparent canvas.
func Fit(src image.Image) *image.RGBA {
	b := src.Bounds()
	den := max(b.Dx(), b.Dy())
	w, h := max(1, side*b.Dx()/den), max(1, side*b.Dy()/den)
	dst := image.NewRGBA(image.Rect(0, 0, side, side))
	x, y := (side-w)/2, (side-h)/2
	draw.ApproxBiLinear.Scale(dst, image.Rect(x, y, x+w, y+h), src, b, draw.Src, nil)
	return dst
}

// ImportPNG returns the same normalized image for preview and local persistence.
func ImportPNG(ctx context.Context, path string) (image.Image, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, "", err
	}
	if !st.Mode().IsRegular() || st.Size() > MaxImportBytes {
		return nil, "", i18n.Error("instance.icon_size_invalid")
	}
	return importPNG(ctx, f)
}

// ImportPNGBytes prepares a PNG already read from a selected local source.
func ImportPNGBytes(ctx context.Context, data []byte) (image.Image, string, error) {
	if len(data) > MaxImportBytes {
		return nil, "", i18n.Error("instance.icon_size_invalid")
	}
	return importPNG(ctx, bytes.NewReader(data))
}

func importPNG(ctx context.Context, src io.ReadSeeker) (image.Image, string, error) {
	cfg, err := png.DecodeConfig(transfer.Reader(ctx, src))
	if err != nil {
		return nil, "", err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > (16<<20)/cfg.Height {
		return nil, "", i18n.Error("instance.icon_size_invalid")
	}
	if _, err = src.Seek(0, io.SeekStart); err != nil {
		return nil, "", err
	}
	decoded, err := png.Decode(transfer.Reader(ctx, io.LimitReader(src, MaxImportBytes)))
	if err != nil {
		return nil, "", err
	}
	img := Fit(decoded)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, "", err
	}
	return img, base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func Decode(encoded string) (image.Image, error) {
	if len(encoded) > 64<<10 {
		return nil, i18n.Error("instance.icon_size_invalid")
	}
	b, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	if cfg.Width != side || cfg.Height != side {
		return nil, i18n.Error("instance.icon_size_invalid")
	}
	return png.Decode(bytes.NewReader(b))
}

package iconimage

import (
	"bytes"
	"context"
	"image"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

const MaxPreviewBytes = 2 << 20

// Preview scales a bounded image to the same canvas as other launcher icons.
func Preview(ctx context.Context, data []byte, requiredFormat string) (image.Image, error) {
	if len(data) > MaxPreviewBytes {
		return nil, i18n.Error("icon.read_failed")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || requiredFormat != "" && requiredFormat != format || config.Width <= 0 || config.Height <= 0 || config.Width > 2048 || config.Height > 2048 {
		return nil, i18n.Error("icon.format_invalid")
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return Fit(src), ctx.Err()
}

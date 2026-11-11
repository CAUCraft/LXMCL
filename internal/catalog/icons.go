package catalog

import (
	"bytes"
	"context"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"github.com/CAUCraft/LXMCL/internal/distribution"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

func catalogIconURL(u *url.URL) bool {
	return u != nil && u.Scheme == "https" && u.User == nil && u.Port() == "" &&
		(u.Hostname() == "cdn.modrinth.com" || distribution.CurseCDN(u))
}

func iconRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 4 || !catalogIconURL(req.URL) {
		return i18n.Error("icon.redirect_denied")
	}
	return nil
}

// LoadIcon reads a bounded PNG, JPEG, GIF or WebP and fits it into a 96px canvas.
func (c Client) LoadIcon(ctx context.Context, raw string) (image.Image, error) {
	u, err := url.Parse(raw)
	if err != nil || !catalogIconURL(u) {
		return nil, i18n.Error("icon.source_invalid")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "CAUCraft/LXMCL (https://github.com/CAUCraft/LXMCL)")
	res, err := c.http(8*time.Second, iconRedirect).Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	const limit = 2 << 20
	if res.StatusCode != http.StatusOK || res.ContentLength > limit {
		return nil, i18n.Error("icon.unavailable")
	}
	data, err := io.ReadAll(io.LimitReader(transfer.Reader(ctx, res.Body), limit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, i18n.Error("icon.read_failed")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width > 2048 || config.Height > 2048 {
		return nil, i18n.Error("icon.format_invalid")
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	const size = 96
	den := max(config.Width, config.Height)
	w, h := max(1, size*config.Width/den), max(1, size*config.Height/den)
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	left, top := (size-w)/2, (size-h)/2
	draw.ApproxBiLinear.Scale(img, image.Rect(left, top, left+w, top+h), src, src.Bounds(), draw.Src, nil)
	return img, ctx.Err()
}

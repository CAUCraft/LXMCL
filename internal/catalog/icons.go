package catalog

import (
	"context"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"time"

	_ "golang.org/x/image/webp"

	"github.com/CAUCraft/LXMCL/internal/distribution"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/iconimage"
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
	const limit = iconimage.MaxPreviewBytes
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
	return iconimage.Preview(ctx, data, "")
}

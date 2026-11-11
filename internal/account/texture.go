package account

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

const maxTextureBytes = 1 << 20

// FetchProfileTexture only reads public assets on the official texture CDN.
// Old profiles may advertise HTTP URLs; use HTTPS for the same texture.
func FetchProfileTexture(ctx context.Context, raw string, skin bool) (image.Image, error) {
	return (Client{}).FetchProfileTexture(ctx, raw, skin)
}

func (c Client) FetchProfileTexture(ctx context.Context, raw string, skin bool) (image.Image, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") ||
		u.Host != "textures.minecraft.net" || u.User != nil || u.RawQuery != "" ||
		u.Fragment != "" || !strings.HasPrefix(u.Path, "/texture/") {
		return nil, i18n.Error("appearance.texture_url_invalid")
	}
	u.Scheme = "https"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	res, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, i18n.Error("appearance.texture_http_status", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxTextureBytes+1))
	if err != nil {
		return nil, err
	}
	return decodeTexture(data, skin)
}

func ReadSkinFile(path string) ([]byte, image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxTextureBytes+1))
	if err != nil {
		return nil, nil, err
	}
	img, err := decodeTexture(data, true)
	if err != nil {
		return nil, nil, err
	}
	return data, img, nil
}

func decodeTexture(data []byte, skin bool) (image.Image, error) {
	if len(data) > maxTextureBytes {
		return nil, i18n.Error("appearance.png_too_large")
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, i18n.Error("appearance.png_invalid")
	}
	if skin && (cfg.Width != 64 || (cfg.Height != 64 && cfg.Height != 32)) {
		return nil, i18n.Error("appearance.skin_dimensions_invalid")
	}
	if !skin && (cfg.Width < 22 || cfg.Height < 17 || cfg.Width > 1024 || cfg.Height > 1024) {
		return nil, i18n.Error("appearance.cape_dimensions_invalid")
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, i18n.Error("appearance.png_incomplete")
	}
	return img, nil
}

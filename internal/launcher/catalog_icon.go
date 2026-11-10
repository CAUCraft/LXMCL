package launcher

import (
	"bytes"
	"context"
	"errors"
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
)

func catalogIconURL(u *url.URL) bool {
	return u != nil && u.Scheme == "https" && u.User == nil && u.Port() == "" &&
		(u.Hostname() == "cdn.modrinth.com" || curseCDN(u))
}

var catalogIconHTTP = &http.Client{
	Timeout: 8 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 4 || !catalogIconURL(req.URL) {
			return errors.New("图标重定向到不支持的来源")
		}
		return nil
	},
}

func LoadCatalogIcon(ctx context.Context, raw string) (image.Image, error) {
	return loadCatalogIcon(ctx, raw, catalogIconHTTP)
}

func loadCatalogIcon(ctx context.Context, raw string, client *http.Client) (image.Image, error) {
	u, err := url.Parse(raw)
	if err != nil || !catalogIconURL(u) {
		return nil, errors.New("不支持的项目图标来源")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "CAUCraft/LXMCL (https://github.com/CAUCraft/LXMCL)")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	const limit = 2 << 20
	if res.StatusCode != http.StatusOK || res.ContentLength > limit {
		return nil, errors.New("项目图标不可用")
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil || len(data) > limit {
		return nil, errors.New("无法读取项目图标")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width > 2048 || config.Height > 2048 {
		return nil, errors.New("项目图标格式或尺寸不受支持")
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
	return img, nil
}

package launcher

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	maxModpackArchive = 1 << 30
	maxModpackFile    = 1 << 30
	maxModpackBytes   = 16 << 30
	maxModpackEntries = 20000
)

type modpackAsset struct {
	URLs              []string
	Size              int64
	Algorithm, Digest string
}

func (a modpackAsset) hash() hash.Hash {
	switch a.Algorithm {
	case "sha512":
		return sha512.New()
	case "sha1":
		return sha1.New()
	case "md5":
		return md5.New()
	}
	return nil
}

func (a modpackAsset) valid(limit int64) bool {
	h := a.hash()
	digest, err := hex.DecodeString(a.Digest)
	return a.Size >= 0 && a.Size <= limit && h != nil && err == nil && len(digest) == h.Size() &&
		slices.ContainsFunc(a.URLs, func(raw string) bool {
			u, err := url.Parse(raw)
			return err == nil && modpackURL(u)
		})
}

func modpackURL(u *url.URL) bool {
	return u.Scheme == "https" && u.User == nil && u.Port() == "" &&
		(slices.Contains([]string{"cdn.modrinth.com", "github.com", "raw.githubusercontent.com", "gitlab.com",
			"release-assets.githubusercontent.com", "objects.githubusercontent.com"}, u.Hostname()) || curseCDN(u))
}

func curseCDN(u *url.URL) bool {
	return u.Scheme == "https" && u.User == nil && u.Port() == "" &&
		slices.Contains([]string{"edge.forgecdn.net", "mediafilez.forgecdn.net", "media.forgecdn.net", "mediafiles.forgecdn.net"}, u.Hostname())
}

var modpackHTTP = &http.Client{
	Timeout: 5 * time.Minute,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 6 || !modpackURL(req.URL) || (curseCDN(via[0].URL) && !curseCDN(req.URL)) {
			return errors.New("整合包重定向超出允许的下载来源")
		}
		return nil
	},
}

type packReader struct {
	ctx context.Context
	r   io.Reader
}

func (r packReader) Read(b []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(b)
}

func (a modpackAsset) copyVerified(ctx context.Context, dst io.Writer, src io.Reader) error {
	h := a.hash()
	n, err := io.Copy(io.MultiWriter(dst, h), io.LimitReader(packReader{ctx, src}, a.Size+1))
	if err != nil {
		return err
	}
	if n != a.Size || !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), a.Digest) {
		return errors.New("下载文件的大小或校验值不匹配")
	}
	return nil
}

func (g Game) modpackAsset(ctx context.Context, a modpackAsset, client *http.Client) (string, error) {
	if !a.valid(maxModpackFile) {
		return "", errors.New("下载文件缺少有效的来源、大小或校验值")
	}
	dir := filepath.Join(g.Cache, "modpacks", a.Algorithm)
	path := filepath.Join(dir, strings.ToLower(a.Digest))
	if f, err := os.Open(path); err == nil {
		err = a.copyVerified(ctx, io.Discard, f)
		f.Close()
		if err == nil {
			return path, nil
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	last := errors.New("没有可用的下载来源")
	for _, raw := range a.URLs {
		u, err := url.Parse(raw)
		if err != nil || !modpackURL(u) {
			continue
		}
		last = retry(ctx, func() error {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
			if err != nil {
				return err
			}
			req.Header.Set("User-Agent", "CAUCraft/LXMCL (https://github.com/CAUCraft/LXMCL)")
			resp, err := client.Do(req)
			if err != nil {
				var urlErr *url.Error
				if errors.As(err, &urlErr) {
					return urlErr.Err
				}
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return &responseError{status: resp.StatusCode, src: u.Hostname(), delay: retryAfter(resp.Header.Get("Retry-After"), time.Now())}
			}
			if resp.ContentLength >= 0 && resp.ContentLength != a.Size {
				return errors.New("下载文件大小与清单不匹配")
			}
			f, err := os.CreateTemp(dir, ".download-*")
			if err != nil {
				return err
			}
			defer os.Remove(f.Name())
			if err := errors.Join(a.copyVerified(ctx, f, resp.Body), f.Close()); err != nil {
				return err
			}
			return os.Rename(f.Name(), path)
		})
		if last == nil {
			return path, nil
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
	}
	return "", fmt.Errorf("无法获取整合包文件：%w", last)
}

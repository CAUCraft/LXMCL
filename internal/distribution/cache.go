package distribution

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

// Cache stores verified files by digest. Root is the application cache directory.
// Its layout intentionally preserves the existing modpacks cache.
type Cache struct {
	Root   string
	Client *http.Client
}

// Ensure returns a file only after verifying its bytes, including cache hits.
// Client may customize transport and timeout; the source and redirect policy is fixed.
func (c Cache) Ensure(ctx context.Context, a Asset) (string, error) {
	if !a.Valid(MaxFileSize) {
		return "", i18n.Error("distribution.metadata_invalid")
	}
	if !filepath.IsAbs(c.Root) {
		return "", i18n.Error("distribution.cache_directory_required")
	}
	configured := c.Client
	if configured == nil {
		configured = defaultHTTP
	}
	// File requests must not inherit an authenticated API client's cookie jar.
	client := http.Client{Transport: configured.Transport, Timeout: configured.Timeout, CheckRedirect: defaultHTTP.CheckRedirect}
	if client.Timeout == 0 {
		client.Timeout = defaultHTTP.Timeout
	}
	dir := filepath.Join(c.Root, "modpacks", a.Algorithm)
	path := filepath.Join(dir, strings.ToLower(a.Digest))
	if f, err := os.Open(path); err == nil {
		err = a.CopyVerified(ctx, io.Discard, f)
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
	last := i18n.Error("distribution.no_source")
	for _, raw := range a.URLs {
		u, err := url.Parse(raw)
		if err != nil || !AllowedURL(u) {
			continue
		}
		last = transfer.Retry(ctx, func() error {
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
				return &transfer.HTTPError{Status: resp.StatusCode, Source: u.Hostname(), Delay: transfer.RetryAfter(resp.Header.Get("Retry-After"), time.Now())}
			}
			if resp.ContentLength >= 0 && resp.ContentLength != a.Size {
				return i18n.Error("distribution.size_mismatch")
			}
			f, err := os.CreateTemp(dir, ".download-*")
			if err != nil {
				return err
			}
			defer os.Remove(f.Name())
			if err := errors.Join(a.CopyVerified(ctx, f, resp.Body), f.Close()); err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
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
	return "", i18n.Error("distribution.fetch_failed", last)
}

package distribution

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

// Fetch streams a validated descriptor from supported file origins, with bounded
// retries and a fixed redirect policy. consume runs once per successful response
// and must verify bytes and publish atomically (e.g. CopyVerified into a temp file).
// It must not retain the reader. Client customizations are limited to transport
// and timeout; API cookies and redirect callbacks are never inherited.
func (a Asset) Fetch(ctx context.Context, configured *http.Client, consume func(context.Context, io.Reader) error) error {
	if !a.Valid(MaxFileSize) {
		return i18n.Error("distribution.metadata_invalid")
	}
	if configured == nil {
		configured = defaultHTTP
	}
	client := http.Client{Transport: configured.Transport, Timeout: configured.Timeout, CheckRedirect: defaultHTTP.CheckRedirect}
	if client.Timeout == 0 {
		client.Timeout = defaultHTTP.Timeout
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
			return consume(ctx, transfer.Reader(ctx, resp.Body))
		})
		if last == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return i18n.Error("distribution.fetch_failed", last)
}

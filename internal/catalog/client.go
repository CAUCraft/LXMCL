package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

// Client queries provider metadata without reading launcher state or writing files.
// The zero value uses the standard HTTP transport. Custom transports must honor
// request contexts; timeouts, credentials and redirect rules remain module-owned.
type Client struct {
	Transport http.RoundTripper
}

func (c Client) http(timeout time.Duration, redirect func(*http.Request, []*http.Request) error) *http.Client {
	return &http.Client{Transport: c.Transport, Timeout: timeout, CheckRedirect: redirect}
}

const maxMetadata = 8 << 20

func (c Client) json(ctx context.Context, raw, key string, payload, dst any) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" ||
		(u.Hostname() != "api.modrinth.com" && u.Hostname() != "api.curseforge.com") ||
		(key != "" && u.Hostname() != "api.curseforge.com") {
		return i18n.Error("catalog.api_source_invalid")
	}
	var body []byte
	method := http.MethodGet
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return err
		}
		method = http.MethodPost
	}
	client := c.http(30*time.Second, func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse })
	return transfer.Retry(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, method, raw, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "CAUCraft/LXMCL (https://github.com/CAUCraft/LXMCL)")
		if key != "" {
			req.Header.Set("x-api-key", key)
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if u.Hostname() == "api.curseforge.com" && (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) {
			return i18n.Error("curseforge.access_denied")
		}
		if resp.StatusCode != http.StatusOK {
			failure := &transfer.HTTPError{Status: resp.StatusCode, Source: u.Hostname()}
			if resp.StatusCode == http.StatusTooManyRequests || resp.Header.Get("Retry-After") != "" {
				failure.Delay = transfer.RetryAfter(resp.Header.Get("Retry-After"), time.Now())
			}
			return failure
		}
		data, err := io.ReadAll(io.LimitReader(transfer.Reader(ctx, resp.Body), maxMetadata+1))
		if err != nil {
			return err
		}
		if len(data) > maxMetadata {
			return i18n.Error("catalog.response_too_large")
		}
		if err := json.Unmarshal(data, dst); err != nil {
			return i18n.Error("catalog.response_invalid", err)
		}
		return ctx.Err()
	})
}

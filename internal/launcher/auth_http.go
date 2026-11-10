package launcher

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

type authClient struct{ http *http.Client }

type authResponseError struct {
	service string
	status  int
	code    string
	xbox    int64
}

func (e *authResponseError) Error() string {
	return i18n.Text("auth.http_status", e.service, e.status)
}

func (c authClient) request(ctx context.Context, service, endpoint, method, contentType string, body io.Reader, bearer string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "CAUCraft/LXMCL (https://github.com/CAUCraft/LXMCL)")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if req.URL.Hostname() == "user.auth.xboxlive.com" || req.URL.Hostname() == "xsts.auth.xboxlive.com" {
		req.Header.Set("x-xbl-contract-version", "1")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return i18n.Error("auth.connection_failed", service, err)
	}
	defer res.Body.Close()
	const limit = 2 << 20
	data, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return i18n.Error("auth.read_failed", service, err)
	}
	if len(data) > limit {
		return i18n.Error("auth.response_too_large", service)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var failure struct {
			Error string `json:"error"`
			XErr  int64  `json:"XErr"`
		}
		_ = json.Unmarshal(data, &failure)
		return &authResponseError{service: service, status: res.StatusCode, code: failure.Error, xbox: failure.XErr}
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return i18n.Error("auth.response_invalid", service)
		}
	}
	return nil
}

func (c authClient) form(ctx context.Context, endpoint string, args url.Values, out any) error {
	args.Set("client_id", microsoftClientID)
	return c.request(ctx, "Microsoft", microsoftOAuth+endpoint, http.MethodPost,
		"application/x-www-form-urlencoded", strings.NewReader(args.Encode()), "", out)
}

func (c authClient) post(ctx context.Context, service, endpoint string, body, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return c.request(ctx, service, endpoint, http.MethodPost, "application/json", bytes.NewReader(raw), "", out)
}

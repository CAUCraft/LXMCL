package javaruntime

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

const catalogURL = "https://piston-meta.mojang.com/v1/products/java-runtime/2ec0cc96c44e5a76b9c8b7c39df7210883d12871/all.json"
const metadataLimit = 8 << 20

var componentID = regexp.MustCompile("^[a-z0-9][a-z0-9-]{0,63}$")

var officialClient = &http.Client{Timeout: 2 * time.Minute, CheckRedirect: func(r *http.Request, via []*http.Request) error {
	if len(via) >= 5 || !allowedURL(r.URL) {
		return i18n.Error("java.runtime_source_invalid")
	}
	return nil
}}

func allowedURL(u *url.URL) bool {
	return u != nil && u.Scheme == "https" && u.User == nil && u.Port() == "" && u.RawQuery == "" && u.Fragment == "" &&
		(u.Hostname() == "piston-meta.mojang.com" || u.Hostname() == "piston-data.mojang.com")
}

func (m Manager) fetch(ctx context.Context, src string, consume func(context.Context, io.Reader) error) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return err
	}
	if !allowedURL(request.URL) {
		return i18n.Error("java.runtime_source_invalid")
	}
	configured := m.Client
	if configured == nil {
		configured = officialClient
	}
	client := *configured
	client.CheckRedirect = officialClient.CheckRedirect
	response, err := client.Do(request)
	if err != nil {
		return canceled(ctx, err)
	}
	defer response.Body.Close()
	if response.Request != nil && !allowedURL(response.Request.URL) {
		return i18n.Error("java.runtime_source_invalid")
	}
	if response.StatusCode != http.StatusOK {
		return i18n.Error("download.http_status", response.StatusCode, src)
	}
	return consume(ctx, response.Body)
}

func (m Manager) catalog(ctx context.Context) (map[string]map[string][]release, error) {
	var result map[string]map[string][]release
	err := m.fetch(ctx, catalogURL, func(ctx context.Context, r io.Reader) error {
		data, err := io.ReadAll(io.LimitReader(r, metadataLimit+1))
		if err != nil {
			return err
		}
		if len(data) > metadataLimit {
			return i18n.Error("metadata.response_too_large")
		}
		return json.Unmarshal(data, &result)
	})
	return result, err
}

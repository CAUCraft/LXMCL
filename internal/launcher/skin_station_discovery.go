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

type stationMetadata struct {
	Meta struct {
		Links struct{ Homepage string }
	}
	SkinDomains []string
}

func stationURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
		u.Fragment != "" || u.Opaque != "" {
		return nil, i18n.Error("station.https_url_required")
	}
	return u, nil
}

func stationRoot(raw string) (*url.URL, error) {
	u, err := stationURL(raw)
	if err != nil || u.RawQuery != "" || u.ForceQuery {
		return nil, i18n.Error("station.api_url_invalid")
	}
	u.Host = strings.ToLower(u.Host)
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
		u.RawPath = ""
	}
	return u, nil
}

func (c authClient) stationMetadata(ctx context.Context, u *url.URL, discover bool) (*url.URL, stationMetadata, []byte, error) {
	var meta stationMetadata
	for hop := 0; hop < 4; hop++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, meta, nil, err
		}
		req.Header.Set("Accept", "application/json")
		res, err := c.http.Do(req)
		if err != nil {
			return nil, meta, nil, i18n.Error("station.connection_failed", err)
		}
		if loc := res.Header.Get("X-Authlib-Injector-API-Location"); discover && loc != "" {
			res.Body.Close()
			ref, err := url.Parse(loc)
			if err != nil {
				return nil, meta, nil, i18n.Error("station.discovery_url_invalid")
			}
			next, err := stationRoot(u.ResolveReference(ref).String())
			if err != nil || next.Host != u.Host {
				return nil, meta, nil, i18n.Error("station.direct_api_required")
			}
			if next.String() != u.String() {
				u = next
				continue
			}
			// Some deployments advertise the API address even at the API root.
			discover = false
			continue
		}
		if res.StatusCode != http.StatusOK {
			res.Body.Close()
			return nil, meta, nil, i18n.Error("station.api_http_status", res.StatusCode)
		}
		const limit = 64 << 10
		data, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
		res.Body.Close()
		if err != nil || len(data) > limit || !bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) ||
			json.Unmarshal(data, &meta) != nil {
			return nil, meta, nil, i18n.Error("station.discovery_api_missing")
		}
		return u, meta, data, nil
	}
	return nil, meta, nil, i18n.Error("station.discovery_loop")
}

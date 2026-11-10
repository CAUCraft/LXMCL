package launcher

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"time"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

const (
	maxMetadata = 8 << 20
	// Verified metadata and small libraries may omit the expected size.
	unknownSize = transfer.UnknownSize
)

var officialHTTP = &http.Client{
	Timeout: 2 * time.Minute,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !officialURL(req.URL) {
			return i18n.Error("download.redirect_denied")
		}
		return nil
	},
}

type downloads struct {
	http            *http.Client
	mu              sync.Mutex
	cooldown        map[string]time.Time
	mavenOriginDown map[string]bool
	mavenMirrorUsed bool
}

func officialURL(u *url.URL) bool {
	return u.Scheme == "https" && u.User == nil && u.Port() == "" &&
		slices.Contains([]string{"piston-meta.mojang.com", "piston-data.mojang.com",
			"libraries.minecraft.net", "resources.download.minecraft.net", "launcher.mojang.com",
			"launchermeta.mojang.com", "meta.fabricmc.net", "maven.fabricmc.net",
			"cdn.modrinth.com", "maven.minecraftforge.net", "files.minecraftforge.net",
			"meta.quiltmc.org", "maven.quiltmc.org",
			"maven.neoforged.net", "repo.maven.apache.org", "repo1.maven.org"}, u.Hostname())
}

func (d *downloads) get(ctx context.Context, src string) (*http.Response, error) {
	return d.getFrom(ctx, src, d.http, officialURL)
}

func (d *downloads) getFrom(ctx context.Context, src string, client *http.Client, allowed func(*url.URL) bool) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return nil, err
	}
	if !allowed(req.URL) {
		return nil, i18n.Error("download.source_invalid", src)
	}
	req.Header.Set("User-Agent", "CAUCraft/LXMCL (https://github.com/CAUCraft/LXMCL)")
	host := req.URL.Hostname()
	for {
		d.mu.Lock()
		delay := time.Until(d.cooldown[host])
		d.mu.Unlock()
		if delay <= 0 {
			break
		}
		if err := waitContext(ctx, delay); err != nil {
			return nil, err
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusOK {
		return resp, nil
	}
	resp.Body.Close()
	failure := &responseError{Status: resp.StatusCode, Source: src}
	if resp.StatusCode == http.StatusTooManyRequests || (resp.StatusCode == http.StatusServiceUnavailable && resp.Header.Get("Retry-After") != "") {
		now := time.Now()
		failure.Delay = retryAfter(resp.Header.Get("Retry-After"), now)
		if failure.Delay <= 30*time.Second {
			until := now.Add(failure.Delay)
			d.mu.Lock()
			if until.After(d.cooldown[host]) {
				if d.cooldown == nil {
					d.cooldown = make(map[string]time.Time)
				}
				d.cooldown[host] = until
			}
			d.mu.Unlock()
		}
	}
	return nil, failure
}

type responseError = transfer.HTTPError

var (
	retryAfter  = transfer.RetryAfter
	waitContext = transfer.Wait
	retryable   = transfer.Retryable
	retry       = transfer.Retry
)

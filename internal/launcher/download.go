package launcher

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
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
			"api.modrinth.com", "cdn.modrinth.com", "maven.minecraftforge.net", "files.minecraftforge.net",
			"meta.quiltmc.org", "maven.quiltmc.org",
			"maven.neoforged.net", "repo.maven.apache.org", "repo1.maven.org"}, u.Hostname())
}

type responseError struct {
	status int
	src    string
	delay  time.Duration
}

func (e *responseError) Error() string {
	if e.status == http.StatusTooManyRequests {
		return i18n.Text("download.rate_limited")
	}
	return i18n.Text("download.http_status", e.status, e.src)
}

func retryAfter(value string, now time.Time) time.Duration {
	if n, err := strconv.Atoi(value); err == nil && n >= 0 {
		// Any value beyond 30s is enough to defer to an explicit user retry.
		return time.Duration(min(n, 31)) * time.Second
	}
	if date, err := http.ParseTime(value); err == nil {
		return max(0, date.Sub(now))
	}
	return time.Second
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
	failure := &responseError{status: resp.StatusCode, src: src}
	if resp.StatusCode == http.StatusTooManyRequests || (resp.StatusCode == http.StatusServiceUnavailable && resp.Header.Get("Retry-After") != "") {
		now := time.Now()
		failure.delay = retryAfter(resp.Header.Get("Retry-After"), now)
		if failure.delay <= 30*time.Second {
			until := now.Add(failure.delay)
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

func waitContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func retryable(err error) bool {
	var response *responseError
	if errors.As(err, &response) {
		return response.delay <= 30*time.Second && (response.status == http.StatusRequestTimeout ||
			response.status == http.StatusTooManyRequests || response.status >= 500 && response.status <= 599)
	}
	var operation *net.OpError
	var timeout net.Error
	return errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, transfer.ErrMismatch) ||
		errors.As(err, &operation) || (errors.As(err, &timeout) && timeout.Timeout())
}

func retry(ctx context.Context, fn func() error) error {
	var err error
	for attempt := range 3 {
		if err := ctx.Err(); err != nil {
			return err
		}
		err = fn()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err == nil || attempt == 2 || !retryable(err) {
			return err
		}
		if err := waitContext(ctx, time.Duration(1<<attempt)*150*time.Millisecond); err != nil {
			return err
		}
	}
	return err
}

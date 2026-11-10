package distribution_test

import (
	"context"
	"crypto/sha512"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/distribution"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func asset(body string) distribution.Asset {
	return distribution.Asset{URLs: []string{"https://cdn.modrinth.com/example.jar"}, Size: int64(len(body)),
		Algorithm: "sha512", Digest: fmt.Sprintf("%x", sha512.Sum512([]byte(body)))}
}

func TestCacheVerifiesReusedBytesAndRepairsCorruption(t *testing.T) {
	const body = "verified file bytes"
	a, calls := asset(body), 0
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(a.URLs[0])
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(u, []*http.Cookie{{Name: "session", Value: "must-stay-in-api-client"}})
	c := distribution.Cache{Root: t.TempDir(), Client: &http.Client{Jar: jar, Transport: transport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("x-api-key") != "" || r.Header.Get("Cookie") != "" {
			t.Fatal("file request contained provider credentials")
		}
		return &http.Response{StatusCode: http.StatusOK, ContentLength: int64(len(body)), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}}
	for n := range 3 {
		name, err := c.Ensure(t.Context(), a)
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(c.Root, "modpacks", "sha512", a.Digest)
		b, err := os.ReadFile(name)
		if err != nil || name != want || string(b) != body {
			t.Fatalf("cache = %q, %q, %v", name, b, err)
		}
		if n == 1 {
			if calls != 1 {
				t.Fatalf("cache hit downloaded again: %d calls", calls)
			}
			if err := os.WriteFile(name, []byte(strings.Repeat("x", len(body))), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	if calls != 2 {
		t.Fatalf("corruption did not cause a verified replacement: %d calls", calls)
	}
}

func TestCacheDoesNotPublishInvalidOrCanceledDownloads(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		t.Run(fmt.Sprint(canceled), func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			a := asset("valid")
			c := distribution.Cache{Root: t.TempDir(), Client: &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
				if canceled {
					cancel()
				}
				return &http.Response{StatusCode: 200, ContentLength: 5, Body: io.NopCloser(strings.NewReader("wrong"))}, nil
			})}}
			if name, err := c.Ensure(ctx, a); err == nil || name != "" || canceled && !errors.Is(err, context.Canceled) {
				t.Fatalf("bad result: %q, %v", name, err)
			}
			entries, err := os.ReadDir(filepath.Join(c.Root, "modpacks", a.Algorithm))
			if err != nil || len(entries) != 0 {
				t.Fatalf("failed download left files: %v, %v", entries, err)
			}
		})
	}
}

func TestCurseForgeRedirectCannotLeaveItsCDN(t *testing.T) {
	a := asset("test")
	a.URLs = []string{"https://edge.forgecdn.net/file.jar"}
	calls := 0
	c := distribution.Cache{Root: t.TempDir(), Client: &http.Client{
		// An injected permissive client must not bypass the distribution policy.
		CheckRedirect: func(*http.Request, []*http.Request) error { return nil },
		Transport: transport(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 302, Header: http.Header{"Location": {"https://cdn.modrinth.com/file.jar"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
		}),
	}}
	if _, err := c.Ensure(t.Context(), a); err == nil || calls != 1 {
		t.Fatalf("redirect escaped policy: %v, %d calls", err, calls)
	}
}

func TestCopyVerifiedRejectsInvalidDescriptors(t *testing.T) {
	for _, a := range []distribution.Asset{
		{}, {Size: math.MaxInt64, Algorithm: "sha512", Digest: asset("").Digest},
		{Size: -1, Algorithm: "sha512", Digest: asset("").Digest},
		{Algorithm: "sha512", Digest: "not-a-digest"},
	} {
		if err := a.CopyVerified(t.Context(), io.Discard, strings.NewReader("")); err == nil {
			t.Fatalf("accepted descriptor: %+v", a)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := asset("").CopyVerified(ctx, io.Discard, strings.NewReader("")); !errors.Is(err, context.Canceled) {
		t.Fatalf("empty file ignored cancellation: %v", err)
	}
}

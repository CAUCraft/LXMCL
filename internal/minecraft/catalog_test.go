package minecraft_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/minecraft"
	"github.com/CAUCraft/LXMCL/internal/testutil/mcfixture"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCatalogRequiresCompleteBoundedJSON(t *testing.T) {
	const valid = `{"versions":[{"id":"1.21.1","type":"release"}]}`
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"valid", valid, true},
		{"exact limit", valid + strings.Repeat(" ", (8<<20)-len(valid)), true},
		{"oversized", valid + strings.Repeat(" ", (8<<20)-len(valid)+1), false},
		{"garbage suffix", valid + "garbage", false},
		{"second object", valid + `{}`, false},
		{"truncated", strings.TrimSuffix(valid, "}"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := minecraft.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.String() != mcfixture.ManifestURL {
					t.Fatalf("unexpected source: %s", r.URL)
				}
				return mcfixture.Response(http.StatusOK, []byte(tc.body)), nil
			})}
			versions, err := client.Releases(t.Context())
			if (err == nil) != tc.valid || calls != 1 || (tc.valid && (len(versions) != 1 || versions[0].ID != "1.21.1")) {
				t.Fatalf("versions=%v err=%v calls=%d", versions, err, calls)
			}
		})
	}
}

func TestCatalogTransportPreservesRedirectPolicyAndCancellation(t *testing.T) {
	calls := 0
	client := minecraft.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		calls++
		resp := mcfixture.Response(http.StatusFound, nil)
		resp.Header.Set("Location", "https://cdn.modrinth.com/not-a-minecraft-catalog")
		return resp, nil
	})}
	if _, err := client.Catalog(t.Context()); err == nil || calls != 1 {
		t.Fatalf("redirect accepted: %v, calls %d", err, calls)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := client.Catalog(ctx); !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("canceled request: %v, calls %d", err, calls)
	}
}

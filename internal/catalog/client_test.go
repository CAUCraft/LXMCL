package catalog

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/modpack"
)

func TestAPIRequestsDoNotFollowRedirects(t *testing.T) {
	for _, provider := range []ModpackSource{ModrinthModpacks, CurseForgeModpacks} {
		t.Run(string(provider), func(t *testing.T) {
			calls := 0
			c := Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				wantKey := ""
				if provider == CurseForgeModpacks {
					wantKey = "private-key"
				}
				if req.Header.Get("x-api-key") != wantKey {
					t.Fatal("wrong provider credentials")
				}
				resp := fixtureResponse(http.StatusFound, nil)
				resp.Header.Set("Location", "https://cdn.modrinth.com/redirect")
				return resp, nil
			})}
			_, err := c.SearchModpacks(t.Context(), provider, "private-key", "", "", 0)
			if err == nil || calls != 1 {
				t.Fatalf("redirect followed: %d requests, %v", calls, err)
			}
			if strings.Contains(err.Error(), "private-key") {
				t.Fatal("error exposed credentials")
			}
		})
	}
}

func TestResolverDoesNotExposeKeyInFormattedOptions(t *testing.T) {
	const key = "private-resolver-api-key"
	resolver := (Client{}).CurseForgeResolver(key)
	options := modpack.InstallOptions{Resolver: resolver}
	for _, format := range []string{"%v", "%+v", "%#v"} {
		if strings.Contains(fmt.Sprintf(format, resolver), key) || strings.Contains(fmt.Sprintf(format, options), key) {
			t.Fatalf("%s exposed resolver credentials", format)
		}
	}
}

func TestCatalogRejectsIncompleteAndOversizedResponses(t *testing.T) {
	for _, body := range []string{
		`{"hits":[{"project_id":"valid","title":"partial"}],"total_hits":"invalid-number"}`,
		`{"hits":[]} trailing`, strings.Repeat(" ", maxMetadata+1),
	} {
		c := Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) { return fixtureResponse(200, []byte(body)), nil })}
		result, err := c.SearchResourceCatalog(t.Context(), ResourceFilter{}, content.Mods, "", "", 0)
		if err == nil || len(result.Hits) != 0 {
			t.Fatalf("invalid document published partial results: %+v, %v", result, err)
		}
	}
}

func TestCatalogWaitsForRateLimitAndHonorsCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		c := Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				resp := fixtureResponse(429, nil)
				resp.Header.Set("Retry-After", "2")
				return resp, nil
			}
			return fixtureResponse(200, []byte(`{"hits":[]}`)), nil
		})}
		start := time.Now()
		_, err := c.SearchResourceCatalog(t.Context(), ResourceFilter{}, content.Mods, "", "", 0)
		if err != nil || calls != 2 || time.Since(start) != 2*time.Second {
			t.Fatalf("rate limit ignored: %d requests, %v, %v", calls, time.Since(start), err)
		}
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		calls = 0
		_, err = c.SearchResourceCatalog(ctx, ResourceFilter{}, content.Mods, "", "", 0)
		if !errors.Is(err, context.DeadlineExceeded) || calls != 1 {
			t.Fatalf("canceled query retried: %d requests, %v", calls, err)
		}
	})
}

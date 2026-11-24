package minecraft_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/minecraft"
	"github.com/CAUCraft/LXMCL/internal/testutil/mcfixture"
)

func TestReleaseCatalogCacheKeepsLastValidatedList(t *testing.T) {
	ctx := t.Context()
	fixture := mcfixture.New(t)
	second := fixture.Version
	second.ID = "1.21.2"
	second.URL = "https://launchermeta.mojang.com/v1/packages/" + second.SHA + "/1.21.2.json"
	manifest := func(entries ...mcfixture.Version) []byte {
		t.Helper()
		body, err := json.Marshal(map[string][]mcfixture.Version{"versions": entries})
		if err != nil {
			t.Fatal(err)
		}
		return body
	}
	legacy := mcfixture.Version{ID: "1.19.2", Type: "release"}
	snapshot := mcfixture.Version{ID: "25w01a", Type: "snapshot"}
	body := manifest(second, legacy, snapshot, fixture.Version)
	status, location, hits := http.StatusOK, "", 0
	var cancelResponse context.CancelFunc
	runtime := minecraft.Runtime{Cache: t.TempDir(), Client: minecraft.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != mcfixture.ManifestURL {
			t.Fatalf("unexpected catalog source: %s", r.URL)
		}
		hits++
		if cancelResponse != nil {
			cancelResponse()
		}
		response := mcfixture.Response(status, body)
		response.Header.Set("Location", location)
		return response, nil
	})}}
	if got, err := runtime.CachedReleases(ctx); len(got) != 0 || err != nil || hits != 0 {
		t.Fatalf("missing local cache fetched or returned releases: %v %v (%d hits)", got, err, hits)
	}
	want := []minecraft.Version{{ID: "1.21.2", Type: "release"}, {ID: "1.21.1", Type: "release"}}
	if got, err := runtime.RefreshReleases(ctx); !slices.Equal(got, want) || err != nil || hits != 1 {
		t.Fatalf("refresh = %v, %v (%d hits)", got, err, hits)
	}
	filename := filepath.Join(runtime.Cache, "official-releases.json")
	assertSaved := func() {
		t.Helper()
		data, err := os.ReadFile(filename)
		if err != nil || string(data) != `{"releases":["1.21.2","1.21.1"]}` {
			t.Fatalf("unexpected cache: %s, %v", data, err)
		}
	}
	assertSaved()
	offline := minecraft.Runtime{Cache: runtime.Cache, Client: minecraft.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
		t.Fatal("reading the saved catalog dispatched a request")
		return nil, nil
	})}}
	if got, err := offline.CachedReleases(ctx); !slices.Equal(got, want) || err != nil {
		t.Fatalf("reopened cache = %v, %v", got, err)
	}

	wrongSource, badSHA := second, second
	wrongSource.URL = "https://meta.fabricmc.net/1.21.2.json"
	badSHA.SHA = "incomplete"
	for _, invalid := range [][]byte{manifest(fixture.Version, wrongSource), manifest(fixture.Version, badSHA), manifest(second, second), []byte("{")} {
		body = invalid
		if got, err := runtime.RefreshReleases(ctx); len(got) != 0 || err == nil {
			t.Fatalf("invalid catalog accepted: %v, %v", got, err)
		}
		assertSaved()
	}
	status, location = http.StatusFound, "https://meta.fabricmc.net/mc/game/version_manifest_v2.json"
	if got, err := runtime.RefreshReleases(ctx); len(got) != 0 || err == nil {
		t.Fatalf("redirect away from Mojang accepted: %v, %v", got, err)
	}
	assertSaved()

	status, body = http.StatusOK, manifest(fixture.Version)
	canceled, cancel := context.WithCancel(ctx)
	defer cancel()
	cancelResponse = cancel
	if got, err := runtime.RefreshReleases(canceled); len(got) != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled refresh accepted: %v, %v", got, err)
	}
	cancelResponse = nil
	assertSaved()

	for _, invalid := range []string{`{"releases":["1.21.1","1.21.1"]}`, `{"releases":["25w01a"]}`, `{"releases":[]}`} {
		if err := os.WriteFile(filename, []byte(invalid), 0600); err != nil {
			t.Fatal(err)
		}
		if got, err := offline.CachedReleases(ctx); len(got) != 0 || err == nil {
			t.Fatalf("invalid local cache accepted: %v, %v", got, err)
		}
	}
	if err := os.Truncate(filename, (8<<20)+1); err != nil {
		t.Fatal(err)
	}
	if got, err := offline.CachedReleases(ctx); len(got) != 0 || err == nil {
		t.Fatalf("oversized local cache accepted: %v, %v", got, err)
	}
	if got, err := runtime.RefreshReleases(ctx); !slices.Equal(got, want[1:]) || err != nil {
		t.Fatalf("refresh did not recover cache: %v, %v", got, err)
	}
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, nil, 0600); err != nil {
		t.Fatal(err)
	}
	runtime.Cache = blocked
	if got, err := runtime.RefreshReleases(ctx); !slices.Equal(got, want[1:]) || err == nil {
		t.Fatalf("saving failure lost fresh list or its error: %v, %v", got, err)
	}
}

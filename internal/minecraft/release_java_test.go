package minecraft_test

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/minecraft"
	"github.com/CAUCraft/LXMCL/internal/testutil/mcfixture"
)

func TestReleaseJavaMajorReadsVerifiedCacheOnly(t *testing.T) {
	const id = "1.20.1"
	r := minecraft.Runtime{Cache: t.TempDir(), Client: minecraft.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
		t.Fatal("verified local metadata required a network request")
		return nil, nil
	})}}
	ctx := t.Context()
	dir := filepath.Join(r.Cache, "versions", id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	write := func(name string, data []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	local := minecraft.Version{ID: id}
	body := []byte(`{"id":"1.20.1","type":"release","javaVersion":{"majorVersion":17}}`)
	digest := sha1.Sum(body)
	write("version.json", body)
	write("verified", []byte("1"))
	if got, err := r.ReleaseJavaMajor(ctx, local); got != 0 || err != nil {
		t.Fatalf("legacy receipt without a checksum returned %d, %v", got, err)
	}
	receipt, err := json.Marshal(map[string]string{
		"platform": runtime.GOOS + "/" + runtime.GOARCH, "version_sha1": hex.EncodeToString(digest[:]),
	})
	if err != nil {
		t.Fatal(err)
	}
	write("verified", receipt)
	if got, err := r.ReleaseJavaMajor(ctx, local); got != 17 || err != nil {
		t.Fatalf("verified cache returned %d, %v", got, err)
	}
	write("version.json", []byte(`{"id":"1.20.1","type":"release","javaVersion":{"majorVersion":21}}`))
	if got, err := r.ReleaseJavaMajor(ctx, local); got != 0 || err != nil {
		t.Fatalf("changed cache returned %d, %v", got, err)
	}
}

func TestReleaseJavaMajorOnlineOnlyReadsSelectedMetadata(t *testing.T) {
	const id = "1.21.1"
	const versionURL = "https://piston-meta.mojang.com/version.json"
	body := []byte(`{"id":"1.21.1","type":"release","javaVersion":{"majorVersion":21}}`)
	digest := sha1.Sum(body)
	release := minecraft.Version{ID: id, Type: "release", URL: versionURL, SHA: hex.EncodeToString(digest[:])}
	var requests []string
	r := minecraft.Runtime{Cache: filepath.Join(t.TempDir(), "cache"), Client: minecraft.Client{Transport: transport(func(req *http.Request) (*http.Response, error) {
		source := req.URL.String()
		requests = append(requests, source)
		switch source {
		case versionURL:
			return mcfixture.Response(http.StatusOK, body), nil
		default:
			t.Fatalf("unexpected download: %s", source)
			return nil, nil
		}
	})}}
	ctx := t.Context()
	if got, err := r.ReleaseJavaMajor(ctx, minecraft.Version{ID: id}); got != 0 || err != nil || len(requests) != 0 {
		t.Fatalf("offline query returned %d, %v and requested %q", got, err, requests)
	}
	if got, err := r.ReleaseJavaMajor(ctx, release); got != 21 || err != nil {
		t.Fatalf("official declaration returned %d, %v", got, err)
	}
	if want := []string{versionURL}; !slices.Equal(requests, want) {
		t.Fatalf("requests = %q, want %q", requests, want)
	}
	body = []byte(`{"id":"1.21.1","type":"release"}`)
	if got, err := r.ReleaseJavaMajor(ctx, release); got != 0 || err == nil {
		t.Fatalf("unverified declaration returned %d, %v", got, err)
	}
	digest = sha1.Sum(body)
	release.SHA = hex.EncodeToString(digest[:])
	if got, err := r.ReleaseJavaMajor(ctx, release); got != 0 || err != nil {
		t.Fatalf("missing declaration returned %d, %v", got, err)
	}
	if _, err := os.Stat(r.Cache); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only query touched the cache: %v", err)
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	before := len(requests)
	if got, err := r.ReleaseJavaMajor(canceled, release); got != 0 || !errors.Is(err, context.Canceled) || len(requests) != before {
		t.Fatalf("canceled lookup returned %d, %v or dispatched a request", got, err)
	}
}

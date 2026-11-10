package launcher

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestNeoForgeFallsBackWithoutAcceptingCorruptBytes(t *testing.T) {
	const path = "net/neoforged/neoforge/21.1.200/neoforge-21.1.200-installer.jar"
	const contents = "verified official jar"
	sha := fmt.Sprintf("%x", sha1.Sum([]byte(contents)))
	var visited []string
	d := &downloads{http: &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		host := req.URL.Hostname()
		visited = append(visited, host+req.URL.Path)
		switch {
		case host == "maven.neoforged.net":
			return nil, context.DeadlineExceeded
		case host == "neoforged.forgecdn.net" && strings.HasSuffix(req.URL.Path, ".sha1"):
			return fixtureResponse(http.StatusOK, []byte(sha+"\n")), nil
		case host == "neoforged.forgecdn.net":
			return fixtureResponse(http.StatusOK, []byte(strings.Repeat("x", len(contents)))), nil
		case host == "bmclapi2.bangbang93.com":
			return fixtureResponse(http.StatusOK, []byte(contents)), nil
		default:
			t.Fatalf("unexpected origin: %s", req.URL)
			return nil, nil
		}
	})}}
	src := neoMaven + path
	actualSHA, err := d.fetchSHA1(t.Context(), src)
	if err != nil || actualSHA != sha {
		t.Fatalf("checksum did not fall back to NeoForge CDN: %q %v", actualSHA, err)
	}
	dest := filepath.Join(t.TempDir(), "installer.jar")
	a := artifact{URL: src, SHA: actualSHA, Size: int64(len(contents))}
	if err := d.fileBound(t.Context(), dest, a, maxMetadata); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(dest)
	if err != nil || string(b) != contents {
		t.Fatalf("unverified mirror file was used: %q %v", b, err)
	}
	if !slices.Contains(visited, "maven.neoforged.net/releases/"+path+".sha1") ||
		slices.Contains(visited, "maven.neoforged.net/releases/"+path) ||
		!slices.Contains(visited, "neoforged.forgecdn.net/releases/"+path) ||
		!slices.Contains(visited, "bmclapi2.bangbang93.com/maven/"+path) {
		t.Fatalf("expected official source, CDN and byte mirror: %v", visited)
	}
}

func TestNeoForgeMirrorsCannotSupplyUntrustedChecksumOrPublishBadFile(t *testing.T) {
	const src = neoMaven + "net/neoforged/neoforge/21.1.200/neoforge-21.1.200-installer.jar"
	d := &downloads{http: &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, ".sha1") {
			if req.URL.Hostname() != "maven.neoforged.net" && req.URL.Hostname() != "neoforged.forgecdn.net" {
				t.Fatal("a general mirror was contacted for a checksum")
			}
			resp := fixtureResponse(http.StatusFound, nil)
			resp.Header.Set("Location", bmclMaven+strings.TrimPrefix(req.URL.Path, "/releases/"))
			return resp, nil
		}
		return fixtureResponse(http.StatusOK, []byte("corrupt")), nil
	})}}
	if sha, err := d.fetchSHA1(t.Context(), src); err == nil || sha != "" {
		t.Fatalf("accepted a checksum through a general mirror: %q %v", sha, err)
	}
	dir := t.TempDir()
	a := artifact{URL: src, SHA: fmt.Sprintf("%x", sha1.Sum([]byte("correct"))), Size: 7}
	if err := d.file(t.Context(), filepath.Join(dir, "installer.jar"), a); !errors.Is(err, errDownloadMismatch) {
		t.Fatalf("expected a checksum failure: %v", err)
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("left unverified artifacts: %v %v", entries, err)
	}
}

func TestNeoForgeCancelStopsBeforeTryingAnotherSource(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var tried int
	d := &downloads{http: &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		tried++
		cancel()
		return nil, context.Canceled
	})}}
	if _, err := d.fetchSHA1(ctx, neoMaven+"net/neoforged/test.jar"); !errors.Is(err, context.Canceled) || tried != 1 {
		t.Fatalf("canceled request still tried other sources: %d %v", tried, err)
	}
}

func TestFabricAndForgeMirrorOnlyVerifiedFiles(t *testing.T) {
	for _, repo := range []string{fabricMaven, forgeMaven} {
		t.Run(repo, func(t *testing.T) {
			const path = "org/example/demo/1.0+build/demo-1.0+build.jar"
			const contents = "verified bytes"
			sha := fmt.Sprintf("%x", sha1.Sum([]byte(contents)))
			d := &downloads{http: &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
				switch req.URL.String() {
				case repo + path + ".sha1":
					return fixtureResponse(http.StatusOK, []byte(sha)), nil
				case repo + path:
					return nil, context.DeadlineExceeded
				case njuMaven + path:
					return fixtureResponse(http.StatusOK, []byte(contents)), nil
				default:
					t.Fatalf("unexpected request: %s", req.URL)
					return nil, nil
				}
			})}}
			actual, err := d.fetchSHA1(t.Context(), repo+path)
			if err != nil || actual != sha {
				t.Fatalf("failed to read origin checksum: %s %v", actual, err)
			}
			dest := filepath.Join(t.TempDir(), "demo.jar")
			if err := d.file(t.Context(), dest, artifact{URL: repo + path, SHA: actual, Size: int64(len(contents))}); err != nil {
				t.Fatal(err)
			}
			if got, err := os.ReadFile(dest); err != nil || string(got) != contents || !d.mavenMirrorUsed {
				t.Fatalf("verified mirror download missing: %q, %v", got, err)
			}
		})
	}
}

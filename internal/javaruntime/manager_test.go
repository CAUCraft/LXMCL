package javaruntime

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/transfer"
)

type transport func(*http.Request) (*http.Response, error)

func (t transport) RoundTrip(r *http.Request) (*http.Response, error) { return t(r) }

func runtimeFixture(t *testing.T) (Manager, Requirement, *bool, *int) {
	t.Helper()
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		t.Skip("managed Windows runtime")
	}
	good := []byte("verified-java")
	asset := transfer.Artifact{URL: "https://piston-data.mojang.com/java", SHA: fmt.Sprintf("%x", sha1.Sum(good)), Size: int64(len(good))}
	f := runtimeFile{Type: "file"}
	f.Downloads.Raw = asset
	manifestData, _ := json.Marshal(manifest{Files: map[string]runtimeFile{"bin/java.exe": f}})
	manifestAsset := transfer.Artifact{URL: "https://piston-meta.mojang.com/manifest", SHA: fmt.Sprintf("%x", sha1.Sum(manifestData)), Size: int64(len(manifestData))}
	catalog, _ := json.Marshal(map[string]map[string][]release{"windows-x64": {"test-runtime": {{Manifest: manifestAsset}}}})
	corrupt, calls := false, 0
	manager := Manager{Root: t.TempDir(), Client: &http.Client{Transport: transport(func(req *http.Request) (*http.Response, error) {
		calls++
		var data []byte
		switch req.URL.String() {
		case catalogURL:
			data = catalog
		case manifestAsset.URL:
			data = manifestData
		case asset.URL:
			data = good
			if corrupt {
				data = []byte("tampered-java")
			}
		default:
			t.Fatalf("unexpected request %s", req.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data)), Request: req}, nil
	})}, probe: func(ctx context.Context, path string) (Runtime, error) {
		if ctx.Err() != nil {
			return Runtime{}, ctx.Err()
		}
		b, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(b, good) {
			return Runtime{}, errors.New("unverified executable")
		}
		return Runtime{Path: path, Major: 25}, nil
	}}
	return manager, Requirement{Component: "test-runtime", Major: 21}, &corrupt, &calls
}

func TestManagedRuntimePublishesVerifiedFilesAndReusesOffline(t *testing.T) {
	m, req, _, calls := runtimeFixture(t)
	j, err := m.Ensure(t.Context(), req, nil)
	if err != nil || j.Major != 25 {
		t.Fatalf("install: %+v %v", j, err)
	}
	if strings.Contains(j.Path, ".install-") {
		t.Fatal("returned staging directory")
	}
	before := *calls
	again, err := m.Ensure(t.Context(), req, nil)
	if err != nil || again != j || *calls != before {
		t.Fatalf("cached install: %+v %v", again, err)
	}
	if err := os.WriteFile(j.Path, []byte("corrupted"), 0600); err != nil {
		t.Fatal(err)
	}
	repaired, err := m.Ensure(t.Context(), req, nil)
	if err != nil || repaired.Path == j.Path || *calls == before {
		t.Fatalf("repair: %+v %v", repaired, err)
	}
}

func TestManagedRuntimeFailureDoesNotPublish(t *testing.T) {
	m, req, corrupt, _ := runtimeFixture(t)
	*corrupt = true
	if _, err := m.Ensure(t.Context(), req, nil); !errors.Is(err, transfer.ErrMismatch) {
		t.Fatalf("checksum: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(m.Root, "windows-x64", req.Component))
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed install left files: %v %v", entries, err)
	}
	*corrupt = false
	ctx, cancel := context.WithCancel(t.Context())
	_, err = m.Ensure(ctx, req, func(done, total int) {
		if done == total {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	entries, _ = os.ReadDir(filepath.Join(m.Root, "windows-x64", req.Component))
	if len(entries) != 0 {
		t.Fatal("canceled runtime published")
	}
}

func TestExplicitJavaNeverDownloads(t *testing.T) {
	m, req, _, calls := runtimeFixture(t)
	_, err := m.Resolve(t.Context(), req, filepath.Join(t.TempDir(), "missing-java.exe"), nil)
	if err == nil || *calls != 0 {
		t.Fatalf("custom Java fallback: %v, %d requests", err, *calls)
	}
}

func TestManifestRejectsTraversalLinksAndCaseCollisions(t *testing.T) {
	for _, files := range []map[string]runtimeFile{
		{"../escape": {Type: "directory"}},
		{"bin/java.exe": {Type: "link"}},
		{"bin": {Type: "directory"}, "BIN": {Type: "directory"}},
		{"manifest.json": {Type: "directory"}},
	} {
		if err := (manifest{Files: files}).validate(); err == nil {
			t.Fatalf("accepted unsafe manifest: %v", files)
		}
	}
}

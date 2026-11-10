package distribution_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/distribution"
)

func TestExportPublishesOnlyVerifiedBytes(t *testing.T) {
	for _, mode := range []string{"success", "checksum", "canceled", "existing file", "late conflict"} {
		t.Run(mode, func(t *testing.T) {
			const body = "verified resource file"
			dir := t.TempDir()
			name := filepath.Join(dir, "file.zip")
			createExisting := func() {
				if err := os.WriteFile(name, []byte("existing"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "existing file" {
				createExisting()
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			calls := 0
			c := distribution.Cache{Root: t.TempDir(), Client: &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
				calls++
				if _, err := os.Lstat(name); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("final name visible before download: %v", err)
				}
				if mode == "canceled" {
					cancel()
				}
				if mode == "late conflict" {
					createExisting()
				}
				data := body
				if mode == "checksum" {
					data = strings.Repeat("x", len(body))
				}
				return &http.Response{StatusCode: 200, ContentLength: int64(len(data)), Body: io.NopCloser(strings.NewReader(data))}, nil
			})}}
			started, completed := 0, 0
			err := c.Export(ctx, asset(body), "file.zip", dir, func(done, total int) {
				if total != 1 {
					t.Errorf("unexpected total: %d", total)
				}
				if done == 0 {
					started++
					return
				}
				completed++
				data, err := os.ReadFile(name)
				if err != nil || string(data) != body {
					t.Fatalf("completed before publication: %q, %v", data, err)
				}
			})
			if mode == "success" {
				if err != nil || completed != 1 || started != 1 || calls != 1 {
					t.Fatalf("export: %v, progress %d/%d, calls %d", err, started, completed, calls)
				}
			} else if err == nil || completed != 0 || mode == "canceled" && !errors.Is(err, context.Canceled) {
				t.Fatalf("unexpected result: %v, completed %d", err, completed)
			}
			if mode == "existing file" && (calls != 0 || started != 0) {
				t.Fatal("existing file triggered a download")
			}
			wantFiles := 0
			switch mode {
			case "success":
				wantFiles = 1
			case "existing file", "late conflict":
				wantFiles = 1
				data, err := os.ReadFile(name)
				if err != nil || string(data) != "existing" {
					t.Fatalf("existing file changed: %q, %v", data, err)
				}
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != wantFiles {
				t.Fatalf("unexpected files: %v, %v", entries, err)
			}
		})
	}
}

func TestExportKeepsChosenDirectory(t *testing.T) {
	const body = "resource"
	parent := t.TempDir()
	dir, moved := filepath.Join(parent, "chosen"), filepath.Join(parent, "moved")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	c := distribution.Cache{Root: t.TempDir(), Client: &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, ContentLength: int64(len(body)), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}}
	target := dir
	err := c.Export(t.Context(), asset(body), "file.zip", dir, func(done, _ int) {
		if done != 0 {
			return
		}
		err := os.Rename(dir, moved)
		// On Windows, os.OpenRoot pins the chosen directory by preventing its move.
		if runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(32)) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(dir, 0755); err != nil {
			t.Fatal(err)
		}
		target = moved
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(target, "file.zip"))
	if err != nil || string(data) != body {
		t.Fatalf("lost chosen directory: %q, %v", data, err)
	}
	if target == moved {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != 0 {
			t.Fatalf("wrote to replacement directory: %v, %v", entries, err)
		}
	}
}

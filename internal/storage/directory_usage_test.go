package storage

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestDirectoryFileUsage(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	cache := filepath.Join(root, "shared", "cache")
	write := func(dir, name, text string) {
		t.Helper()
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(root, "empty", "")
	write(filepath.Join(root, "mods"), "three", "123")
	write(cache, "ignored", "shared cache")
	write(outside, "ignored", "outside the instance")
	link := filepath.Join(root, "external")
	linked := os.Symlink(outside, link) == nil
	if !linked {
		t.Log("directory links unavailable on this test host")
	}
	stats, err := DirectoryFileUsage(t.Context(), root, cache)
	if err != nil || stats != (FileUsage{Files: 2, Bytes: 3}) {
		t.Fatalf("instance usage = %+v, error = %v", stats, err)
	}
	if linked {
		if stats, err := DirectoryFileUsage(t.Context(), link, cache); !errors.Is(err, fs.ErrInvalid) || stats != (FileUsage{}) {
			t.Fatalf("linked instance usage = %+v, error = %v", stats, err)
		}
	}
	if stats, err := DirectoryFileUsage(t.Context(), cache, cache); !errors.Is(err, fs.ErrInvalid) || stats != (FileUsage{}) {
		t.Fatalf("shared cache reported as an instance: %+v, error = %v", stats, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if stats, err := DirectoryFileUsage(ctx, root, cache); !errors.Is(err, context.Canceled) || stats != (FileUsage{}) {
		t.Fatalf("canceled usage = %+v, error = %v", stats, err)
	}

	blocked := filepath.Join(root, "blocked")
	if err := os.Mkdir(blocked, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(blocked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0700) })
	if _, err := os.ReadDir(blocked); !errors.Is(err, fs.ErrPermission) {
		t.Log("directory access restrictions do not apply to the test account")
		return
	}
	if stats, err := DirectoryFileUsage(t.Context(), root, cache); !errors.Is(err, fs.ErrPermission) || stats != (FileUsage{}) {
		t.Fatalf("incomplete usage = %+v, error = %v", stats, err)
	}
}

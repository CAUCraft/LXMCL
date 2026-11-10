package storage

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublishNewCommitsPreparedFile(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	err = PublishNew(context.Background(), root, "资源.zip", 0644, func(f *os.File) error {
		if _, err := root.Lstat("资源.zip"); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("final file visible during preparation: %v", err)
		}
		_, err := f.WriteString("verified contents")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "资源.zip"))
	if err != nil || string(data) != "verified contents" {
		t.Fatalf("published %q: %v", data, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("unexpected staging files: %v, %v", entries, err)
	}
}

func TestPublishNewFailureLeavesNoFinalFile(t *testing.T) {
	rejected := errors.New("validation failed")
	for _, mode := range []string{"validation", "canceled", "closed writer"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			err = PublishNew(ctx, root, "file.zip", 0644, func(f *os.File) error {
				if _, err := f.WriteString("partial"); err != nil {
					return err
				}
				switch mode {
				case "validation":
					return rejected
				case "canceled":
					cancel()
				case "closed writer":
					return f.Close()
				}
				return nil
			})
			if err == nil || mode == "validation" && !errors.Is(err, rejected) || mode == "canceled" && !errors.Is(err, context.Canceled) {
				t.Fatalf("unexpected failure: %v", err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 0 {
				t.Fatalf("failed publication left files: %v, %v", entries, err)
			}
		})
	}
}

func TestPublishNewRejectsInvalidNamesAndCanceledWork(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	prepare := func(*os.File) error { t.Fatal("preparation should not start"); return nil }
	for _, name := range []string{"", ".", "..", "../file.zip", "folder/file.zip", `folder\file.zip`, "/file.zip", `C:\file.zip`, "CON.zip", "file.zip.", strings.Repeat("x", 256)} {
		if err := PublishNew(t.Context(), root, name, 0644, prepare); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := PublishNew(ctx, root, "file.zip", 0644, prepare); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestPublishNewKeepsOpenedDirectoryWhenMoved(t *testing.T) {
	parent := t.TempDir()
	dir, moved := filepath.Join(parent, "chosen"), filepath.Join(parent, "moved")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	parentRoot, err := os.OpenRoot(parent)
	if err != nil {
		t.Fatal(err)
	}
	defer parentRoot.Close()
	root, err := parentRoot.OpenRoot("chosen")
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	// Move the opened directory before creating the staged file. Windows may
	// prevent a directory move while one of its child files is open for writing.
	if err := os.Rename(dir, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	err = PublishNew(t.Context(), root, "file.zip", 0644, func(f *os.File) error {
		_, err := f.WriteString("original directory")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(moved, "file.zip"))
	if err != nil || string(data) != "original directory" {
		t.Fatalf("lost original target: %q, %v", data, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("wrote into replacement directory: %v, %v", entries, err)
	}
}

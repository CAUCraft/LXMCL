package storage

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
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
	for _, mode := range []string{"validation", "canceled"} {
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

func TestPublishNewPreservesFileCreatedDuringPreparation(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit conflict", true: "validation failure"}[rejected], func(t *testing.T) {
			dir := t.TempDir()
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			failure := errors.New("invalid archive")
			err = PublishNew(t.Context(), root, "file.zip", 0644, func(f *os.File) error {
				if _, err := f.WriteString("new file"); err != nil {
					return err
				}
				if err := os.WriteFile(filepath.Join(dir, "file.zip"), []byte("another writer"), 0644); err != nil {
					return err
				}
				if rejected {
					return failure
				}
				return nil
			})
			want := error(fs.ErrExist)
			if rejected {
				want = failure
			}
			if !errors.Is(err, want) {
				t.Fatalf("unexpected result: %v", err)
			}
			data, err := os.ReadFile(filepath.Join(dir, "file.zip"))
			if err != nil || string(data) != "another writer" {
				t.Fatalf("concurrent file changed: %q, %v", data, err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 {
				t.Fatalf("unexpected files: %v, %v", entries, err)
			}
		})
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

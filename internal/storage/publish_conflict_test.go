package storage

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestPublishNewPreservesExistingEntries(t *testing.T) {
	for _, entry := range []string{"file", "directory", "symlink"} {
		t.Run(entry, func(t *testing.T) {
			dir := t.TempDir()
			name := filepath.Join(dir, "file.zip")
			switch entry {
			case "file":
				if err := os.WriteFile(name, []byte("existing"), 0644); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(name, 0755); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink(filepath.Join(dir, "absent"), name); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			}
			before, err := os.Lstat(name)
			if err != nil {
				t.Fatal(err)
			}
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			err = PublishNew(t.Context(), root, "file.zip", 0644, func(*os.File) error { t.Fatal("existing entry was ignored"); return nil })
			if !errors.Is(err, fs.ErrExist) {
				t.Fatalf("expected conflict: %v", err)
			}
			after, err := os.Lstat(name)
			if err != nil || !os.SameFile(before, after) {
				t.Fatalf("entry replaced: %v", err)
			}
			if entry == "file" {
				data, err := os.ReadFile(name)
				if err != nil || string(data) != "existing" {
					t.Fatalf("existing file changed: %q, %v", data, err)
				}
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

func TestConcurrentPublishersCommitExactlyOneFile(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	prepared, release := make(chan struct{}, 2), make(chan struct{})
	results := make(chan error, 2)
	for _, data := range []string{"first complete file", "second complete file"} {
		go func() {
			results <- PublishNew(t.Context(), root, "file.zip", 0644, func(f *os.File) error {
				if _, err := f.WriteString(data); err != nil {
					return err
				}
				prepared <- struct{}{}
				<-release
				return nil
			})
		}()
	}
	for range 2 {
		select {
		case <-prepared:
		case err := <-results:
			close(release)
			t.Fatalf("publisher failed before preparation: %v", err)
		}
	}
	close(release)
	successes := 0
	for range 2 {
		err := <-results
		if err == nil {
			successes++
		} else if !errors.Is(err, fs.ErrExist) {
			t.Errorf("unexpected conflict: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("committed %d files", successes)
	}
	data, err := os.ReadFile(filepath.Join(dir, "file.zip"))
	if err != nil || string(data) != "first complete file" && string(data) != "second complete file" {
		t.Fatalf("incomplete file: %q, %v", data, err)
	}
}

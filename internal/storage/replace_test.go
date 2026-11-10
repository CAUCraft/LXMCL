package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceFileKeepsOldContentUntilCommit(t *testing.T) {
	for _, mode := range []string{"rejected", "canceled", "committed"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "cache")
			if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			rejected := errors.New("invalid bytes")
			err = ReplaceFile(ctx, root, "cache", 0600, func(file *os.File) error {
				if _, err := file.WriteString("replacement"); err != nil {
					return err
				}
				if data, err := os.ReadFile(path); err != nil || string(data) != "original" {
					t.Fatalf("changed destination during preparation: %q, %v", data, err)
				}
				if mode == "rejected" {
					return rejected
				}
				if mode == "canceled" {
					cancel()
				}
				return nil
			})
			want, contents := error(nil), "original"
			switch mode {
			case "rejected":
				want = rejected
			case "canceled":
				want = context.Canceled
			case "committed":
				contents = "replacement"
			}
			if !errors.Is(err, want) {
				t.Fatalf("replacement result: %v, want %v", err, want)
			}
			if data, err := os.ReadFile(path); err != nil || string(data) != contents {
				t.Fatalf("destination = %q, want %q: %v", data, contents, err)
			}
			if entries, err := os.ReadDir(dir); err != nil || len(entries) != 1 {
				t.Fatalf("left temporary files: %v, %v", entries, err)
			}
		})
	}
}

package screenshots

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestOriginalRevealPathRequiresSamePNG(t *testing.T) {
	game := t.TempDir()
	dir := filepath.Join(game, Directory)
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	name := "河谷 截图.PNG"
	path := filepath.Join(dir, name)
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, image.NewGray(image.Rect(0, 0, 5, 5))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, imageBytes.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	_, original, err := Preview(t.Context(), game, name)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := original.RevealPath(t.Context()); got != path || err != nil {
		t.Fatalf("original path = %q, %v, want %q", got, err, path)
	}
	if err := os.Rename(path, filepath.Join(dir, "old.png")); err != nil {
		t.Fatal(err)
	}
	if got, err := original.RevealPath(t.Context()); got != "" || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing path = %q, %v", got, err)
	}
	if err := os.WriteFile(path, imageBytes.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	at := original.info.ModTime()
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
	if got, err := original.RevealPath(t.Context()); got != "" || err == nil {
		t.Fatalf("accepted a new file with the same name, bytes and timestamp: %q", got)
	}
	_, refreshed, err := Preview(t.Context(), game, name)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := refreshed.RevealPath(t.Context()); got != path || err != nil {
		t.Fatalf("refreshed path = %q, %v, want %q", got, err, path)
	}
	t.Run("configured game path", func(t *testing.T) {
		alias := filepath.Join(t.TempDir(), "linked instance")
		if err := os.Symlink(game, alias); err != nil {
			t.Skipf("directory symlink unavailable: %v", err)
		}
		_, selected, err := Preview(t.Context(), alias, name)
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(alias, Directory, name)
		if got, err := selected.RevealPath(t.Context()); got != want || err != nil {
			t.Fatalf("configured path = %q, %v, want %q", got, err, want)
		}
	})
}

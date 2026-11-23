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
	"time"
)

func TestGalleryReadsOnlyInstancePNGsInTimeOrder(t *testing.T) {
	game := t.TempDir()
	dir := filepath.Join(game, Directory)
	if _, err := List(t.Context(), game); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"early.png", "later.PNG", "ignored.txt"} {
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, image.NewRGBA(image.Rect(0, 0, 2000, 250)))
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	at := time.Unix(1700000000, 0)
	if err := os.Chtimes(filepath.Join(dir, "early.png"), at, at); err != nil {
		t.Fatal(err)
	}
	rows, err := List(t.Context(), game)
	if err != nil || len(rows) != 2 || rows[0].Name != "later.PNG" || rows[1].Name != "early.png" {
		t.Fatal(rows, err)
	}
	img, original, err := Preview(t.Context(), game, rows[0].Name)
	if err != nil || original.Size != image.Pt(2000, 250) || img.Bounds().Size() != image.Pt(1600, 200) {
		t.Fatal(original, err)
	}
	folder := t.TempDir()
	if err := Export(t.Context(), game, rows[0].Name, folder); err != nil {
		t.Fatal(err)
	}
	read := func(path string) []byte {
		t.Helper()
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	full, err := original.PNG(t.Context())
	if err != nil || !bytes.Equal(full, read(filepath.Join(dir, rows[0].Name))) {
		t.Fatal("clipboard image differs from the original 2000-pixel PNG", err)
	}
	out := filepath.Join(folder, rows[0].Name)
	if !bytes.Equal(read(out), read(filepath.Join(dir, rows[0].Name))) {
		t.Fatal("export differs from original 2000-pixel PNG")
	}
	if err := os.WriteFile(out, []byte("previous export"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Export(t.Context(), game, rows[0].Name, folder); err == nil || string(read(out)) != "previous export" {
		t.Fatal("existing export replaced", err)
	}
	if _, _, err := Preview(t.Context(), t.TempDir(), rows[0].Name); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
}

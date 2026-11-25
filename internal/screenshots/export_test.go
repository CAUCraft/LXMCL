package screenshots

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBatchExport(t *testing.T) {
	game := t.TempDir()
	dir := filepath.Join(game, Directory)
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	const a, b, c = "营地-A.png", "营地-B.PNG", "营地-C.png"
	data := make(map[string][]byte)
	at := time.Unix(1700000000, 0)
	for i, name := range []string{a, b, c} {
		var raw bytes.Buffer
		img := image.NewRGBA(image.Rect(0, 0, 3+i, 2))
		img.Set(0, 0, color.RGBA{R: uint8(9 + i), A: 255})
		if err := png.Encode(&raw, img); err != nil {
			t.Fatal(err)
		}
		data[name] = raw.Bytes()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data[name], 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := List(t.Context(), game)
	if err != nil || len(entries) != 3 || entries[0].Name != a || entries[1].Name != b || entries[2].Name != c {
		t.Fatal(entries, err)
	}
	out := t.TempDir()
	sentinel := []byte("existing screenshot")
	if err := os.WriteFile(filepath.Join(out, b), sentinel, 0600); err != nil {
		t.Fatal(err)
	}
	check := func(folder, name string, want []byte) {
		t.Helper()
		got, err := os.ReadFile(filepath.Join(folder, name))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("%s: got %d bytes, %v", name, len(got), err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result, err := ExportBatch(ctx, game, entries, out, func(_ string, done int) {
		if done == 2 {
			cancel()
		}
	})
	if result != (BatchExport{Exported: 1, Skipped: 1}) || !errors.Is(err, context.Canceled) {
		t.Fatal(result, err)
	}
	if files, err := os.ReadDir(out); err != nil || len(files) != 2 {
		t.Fatal("cancellation left an extra file", files, err)
	}
	check(out, a, data[a])
	check(out, b, sentinel)
	sep := string(filepath.Separator)
	if result, err := ExportBatch(t.Context(), game+sep, entries, out+sep, nil); err != nil || result != (BatchExport{Exported: 1, Skipped: 2}) {
		t.Fatal(result, err)
	}
	check(out, c, data[c])
	for _, name := range []string{a, b, c} {
		check(dir, name, data[name])
	}

	// A newly created file can have the original name, bytes and timestamp.
	// The captured directory entry still identifies the original file.
	path := filepath.Join(dir, c)
	if err := os.Rename(path, filepath.Join(dir, "kept-original.png")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data[c], 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
	fresh := t.TempDir()
	if result, err := ExportBatch(t.Context(), game, entries, fresh, nil); err == nil || result != (BatchExport{Exported: 2, Failed: 1}) {
		t.Fatal(result, err)
	}
	if _, err := os.Lstat(filepath.Join(fresh, c)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("stale selection was exported", err)
	}

	t.Run("links", func(t *testing.T) {
		realParent, linkParent := t.TempDir(), filepath.Join(t.TempDir(), "linked")
		actual := filepath.Join(realParent, "out")
		if err := os.Mkdir(actual, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(realParent, linkParent); err != nil {
			t.Skipf("cannot create test directory link: %v", err)
		}
		if got, err := ExportBatch(t.Context(), game, entries[:1], filepath.Join(linkParent, "out"), nil); err == nil || got != (BatchExport{}) {
			t.Fatal("export traversed a linked destination", got, err)
		}
		if files, err := os.ReadDir(actual); err != nil || len(files) != 0 {
			t.Fatal("linked destination was written", files, err)
		}
		original := filepath.Join(dir, a)
		link := filepath.Join(actual, a)
		if err := os.Symlink(original, link); err != nil {
			t.Skipf("cannot create test file link: %v", err)
		}
		if got, err := ExportBatch(t.Context(), game, entries[:1], actual, nil); err != nil || got != (BatchExport{Skipped: 1}) {
			t.Fatal("linked destination file was not skipped", got, err)
		}
		check(dir, a, data[a])
		if err := os.Remove(link); err != nil {
			t.Fatal(err)
		}
		movedFile := filepath.Join(dir, "kept-a.png")
		if err := os.Rename(original, movedFile); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(movedFile, original); err != nil {
			t.Fatal(err)
		}
		if got, err := ExportBatch(t.Context(), game, entries[:1], actual, nil); err == nil || got != (BatchExport{Failed: 1}) {
			t.Fatal("export traversed a linked source file", got, err)
		}
		moved := filepath.Join(game, "actual")
		if err := os.Rename(dir, moved); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(moved, dir); err != nil {
			t.Fatal(err)
		}
		if got, err := ExportBatch(t.Context(), game, entries[:1], actual, nil); err == nil || got != (BatchExport{}) {
			t.Fatal("export traversed a linked source directory", got, err)
		}
		if files, err := os.ReadDir(actual); err != nil || len(files) != 0 {
			t.Fatal("linked source was exported", files, err)
		}
	})
}

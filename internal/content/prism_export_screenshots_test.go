package content

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/tasks"
)

func TestPrismExportScreenshots(t *testing.T) {
	ctx := t.Context()
	i := instance.Instance{Name: "Island", Version: "1.21.4", GameDir: t.TempDir()}
	out := t.TempDir()
	write := func(name string, data []byte) {
		t.Helper()
		path := filepath.Join(i.GameDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	makePNG := func(width int) []byte {
		t.Helper()
		var b bytes.Buffer
		if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, width, 2))); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	export := func(name string, selected bool) string {
		t.Helper()
		path := filepath.Join(out, name+".zip")
		files := PrismFiles{Screenshots: selected}
		record, err := ExportPrismTaskZIP(ctx, i, path, files, nil, nil, nil)
		if err != nil || record.Path != path {
			t.Fatal(record, err)
		}
		return path
	}
	read := func(name string) []byte {
		t.Helper()
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}

	baseline := read(export("missing", true))
	pngs := map[string][]byte{"screenshots/岛屿.png": makePNG(2), "screenshots/bridge.PNG": makePNG(3)}
	for name, data := range pngs {
		write(name, data)
	}
	for _, name := range []string{"screenshots/note.txt", "screenshots/nested/world.png", "logs/latest.log", "accounts.json"} {
		write(name, makePNG(4))
	}
	if actual := read(export("default", false)); !bytes.Equal(actual, baseline) {
		t.Fatal("default ZIP changed after adding screenshots")
	}

	filename := export("with-screenshots", true)
	preview, err := ReadPrismZIP(ctx, filename)
	if err != nil || preview.Name != i.Name || preview.Version != i.Version {
		t.Fatal(preview, err)
	}
	if err := CopyPrismFiles(ctx, preview, filepath.Join(t.TempDir(), "imported"), PrismFiles{}, func() error { return nil }, nil); err != nil {
		t.Fatal(err)
	}
	z, err := zip.OpenReader(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	if len(z.File) != len(pngs)+2 {
		t.Fatalf("unexpected ZIP member count: %d", len(z.File))
	}
	for name, original := range pngs {
		picture, err := z.Open("instance/.minecraft/" + name)
		if err != nil {
			t.Fatal(err)
		}
		archived, err := io.ReadAll(picture)
		picture.Close()
		if err != nil || !bytes.Equal(archived, original) || !bytes.Equal(read(filepath.Join(i.GameDir, filepath.FromSlash(name))), original) {
			t.Fatal("PNG bytes changed", name, err)
		}
	}

	cancelled := filepath.Join(out, "cancelled.zip")
	exportCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	record, err := ExportPrismTaskZIP(exportCtx, i, cancelled, PrismFiles{Screenshots: true}, nil, nil, func(p tasks.Progress) {
		if p.Done == 2 {
			cancel()
		}
	})
	if record.Path != "" || !errors.Is(err, context.Canceled) {
		t.Fatal(record, err)
	}
	if entries, err := os.ReadDir(out); err != nil || len(entries) != 3 {
		t.Fatal("cancel left output or temporary files", entries, err)
	}
}

func TestPrismExportScreenshotsSkipsFileLinksAndRejectsLinkedRoot(t *testing.T) {
	dir, external, out := t.TempDir(), t.TempDir(), t.TempDir()
	screens := filepath.Join(dir, "screenshots")
	if err := os.Mkdir(screens, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(external, "external.png")
	if err := os.WriteFile(target, []byte("outside instance"), 0600); err != nil {
		t.Fatal(err)
	}
	link := func(target, name string) {
		t.Helper()
		if err := os.Symlink(target, name); err != nil {
			if os.PathSeparator == '\\' {
				t.Skip("Windows runner cannot create symbolic links:", err)
			}
			t.Fatal(err)
		}
	}
	link(target, filepath.Join(screens, "follow.png"))
	i := instance.Instance{Name: "Island", Version: "1.21.4", GameDir: dir}
	files := PrismFiles{Screenshots: true}
	first := filepath.Join(out, "only-file-link.zip")
	if err := ExportPrismFiles(t.Context(), i, first, files, nil); err != nil {
		t.Fatal(err)
	}
	z, err := zip.OpenReader(first)
	if err != nil {
		t.Fatal(err)
	}
	z.Close()
	if len(z.File) != 2 {
		t.Fatal("ZIP contained a linked screenshot")
	}
	if err := os.Remove(filepath.Join(screens, "follow.png")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(screens); err != nil {
		t.Fatal(err)
	}
	link(external, screens)
	if err := ExportPrismFiles(t.Context(), i, filepath.Join(out, "linked-root.zip"), files, nil); err == nil {
		t.Fatal("export accepted a linked screenshot directory")
	}
	if entries, err := os.ReadDir(out); err != nil || len(entries) != 1 {
		t.Fatal("failed export left output or temporary files", entries, err)
	}
}

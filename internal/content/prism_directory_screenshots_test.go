package content

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/tasks"
)

func TestPrismDirectoryImportScreenshots(t *testing.T) {
	ctx := t.Context()
	write := func(base, name string, data []byte) {
		t.Helper()
		file := filepath.Join(base, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	picture := func(width int) []byte {
		t.Helper()
		var out bytes.Buffer
		if err := png.Encode(&out, image.NewRGBA(image.Rect(0, 0, width, 2))); err != nil {
			t.Fatal(err)
		}
		return out.Bytes()
	}
	makeSource := func(folder string, data []byte) string {
		t.Helper()
		source := t.TempDir()
		write(source, "instance.cfg", []byte("name=Island\n"))
		write(source, "mmc-pack.json", []byte(`{"formatVersion":1,"components":[{"uid":"net.minecraft","version":"1.21.4"}]}`))
		write(source, folder+"/screenshots/岛屿.PNG", data)
		return source
	}
	a, b := picture(2), picture(3)
	first, second := makeSource(".minecraft", a), makeSource("minecraft", b)
	write(second, "minecraft/screenshots/harbor.png", picture(4))
	write(second, "minecraft/screenshots/nested/ignored.png", picture(5))
	write(second, "minecraft/screenshots/note.txt", []byte("unselected text"))
	write(second, "minecraft/logs/latest.log", []byte("unselected log"))
	write(second, "accounts.json", []byte("{}"))
	preview := func(source string) PrismInstance {
		t.Helper()
		p, err := ReadPrismInstance(ctx, source)
		if err != nil || !p.Screenshots {
			t.Fatal("expected a screenshot option", p, err)
		}
		return p
	}
	pa, pb := preview(first), preview(second)
	parent := t.TempDir()
	commit := func() error { return nil }
	if err := CopyPrismFiles(ctx, pa, filepath.Join(parent, "default"), PrismFiles{}, commit, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(parent, "default", "screenshots")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("unchecked import created screenshots", err)
	}
	selected := PrismFiles{Screenshots: true}
	target := filepath.Join(parent, "selected")
	if err := CopyPrismFiles(ctx, pb, target, selected, commit, nil); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(target, "screenshots")
	if entries, err := os.ReadDir(destination); err != nil || len(entries) != 2 {
		t.Fatal("imported unexpected screenshot paths", entries, err)
	}
	if entries, err := os.ReadDir(target); err != nil || len(entries) != 1 {
		t.Fatal("imported unrelated files", entries, err)
	}
	for _, check := range []struct {
		name string
		want []byte
	}{
		{filepath.Join(destination, "岛屿.PNG"), b},
		{filepath.Join(second, "minecraft", "screenshots", "岛屿.PNG"), b},
		{filepath.Join(first, ".minecraft", "screenshots", "岛屿.PNG"), a},
	} {
		if got, err := os.ReadFile(check.name); err != nil || !bytes.Equal(got, check.want) {
			t.Fatal("wrong original or destination image", check.name, err)
		}
	}

	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	noCommit := func() error { t.Fatal("failed import committed"); return nil }
	err := CopyPrismFiles(cancelCtx, pb, filepath.Join(parent, "cancelled"), selected, noCommit, func(p tasks.Progress) {
		if p.Done == 1 {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatal("expected cancelled screenshot copy", err)
	}
	write(second, "minecraft/screenshots/岛屿.PNG", picture(11))
	if err := CopyPrismFiles(ctx, pb, filepath.Join(parent, "changed"), selected, noCommit, nil); err == nil {
		t.Fatal("accepted an original that changed after preview")
	}
	if entries, err := os.ReadDir(parent); err != nil || len(entries) != 2 {
		t.Fatal("failed imports left a destination or a staging directory", entries, err)
	}

	t.Run("linked gallery is unavailable", func(t *testing.T) {
		gallery := filepath.Join(second, "minecraft", "screenshots")
		if err := os.Rename(gallery, filepath.Join(second, "saved-gallery")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(first, ".minecraft", "screenshots"), gallery); err != nil {
			if os.PathSeparator == '\\' {
				t.Skip("Windows runner cannot create symbolic links:", err)
			}
			t.Fatal(err)
		}
		p, err := ReadPrismInstance(t.Context(), second)
		if err != nil || p.Screenshots {
			t.Fatal("directory import offered linked screenshots", p, err)
		}
		if err := CopyPrismFiles(t.Context(), p, filepath.Join(parent, "linked-default"), PrismFiles{}, commit, nil); err != nil {
			t.Fatal("unselected linked gallery prevented importing the profile", err)
		}
	})
}

package content_test

import (
	"archive/zip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/tasks"
)

func TestRestoreSelectedMods(t *testing.T) {
	src := instance.Instance{Loader: "Fabric", LoaderVersion: "0.16.14", Version: "1.21.1", GameDir: t.TempDir()}
	target := src
	target.GameDir = t.TempDir()
	archive := filepath.Join(t.TempDir(), "mods.zip")
	names := []string{"a.jar", "b.jar.disabled", "c.jar", "d.jar", "e.jar", "f.jar", "g.jar"}
	for j, name := range names {
		id := []string{"apple", "bee", "clover", "dove", "dove", "elm", "fir"}[j]
		writeTestMod(t, src, name, "fabric.mod.json", `{"id":"`+id+`"}`)
	}
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	add := func(name string, b []byte) {
		t.Helper()
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write(b); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range names {
		b, err := os.ReadFile(filepath.Join(src.GameDir, "mods", name))
		if err != nil {
			t.Fatal(err)
		}
		add("mods/"+name, b)
	}
	add("mods/h.jar", []byte("unfinished local file"))
	add("mods/readme.txt", []byte("local notes"))
	add("more/other.jar", []byte("not selected"))
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	writeTestMod(t, target, "renamed.jar", "fabric.mod.json", `{"id":"clover"}`)
	writeTestMod(t, target, "f.jar.disabled", "fabric.mod.json", `{"id":"different"}`)
	p, err := content.PreviewModRestore(t.Context(), target, archive)
	if err != nil || len(p.Items) != 8 {
		t.Fatal(p, err)
	}
	selected := map[string]bool{}
	for _, item := range p.Items {
		name := item.File.DiskName()
		allowed := name == "a.jar" || name == "b.jar.disabled" || name == "g.jar"
		if (item.Conflict == "") != allowed {
			t.Fatal(item)
		}
		selected[name] = true
	}
	// A conflict that arrived after preview also keeps its original file.
	writeTestMod(t, target, "late.jar", "fabric.mod.json", `{"id":"fir"}`)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	n, _, err := p.Restore(ctx, selected, func(s tasks.Progress) {
		if s.Done == 1 {
			cancel()
		}
	})
	if n != 1 || !errors.Is(err, context.Canceled) {
		t.Fatal(n, err)
	}
	n, skipped, err := p.Restore(t.Context(), selected, nil)
	if n != 1 || skipped != 7 || err != nil {
		t.Fatal(n, skipped, err)
	}
	installed, err := content.Inventory(t.Context(), target, content.Mods)
	if err != nil || len(installed) != 5 {
		t.Fatal(installed, err)
	}
	for _, f := range installed {
		if f.Name == "b.jar" && f.Enabled {
			t.Fatal("lost disabled state")
		}
	}
	if err := os.Rename(archive, archive+".old"); err != nil {
		t.Fatal(err)
	}
	n, _, err = p.Restore(t.Context(), selected, nil)
	if n != 0 || err == nil {
		t.Fatal("missing backup", n, err)
	}
}

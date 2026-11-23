package content_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/tasks"
)

func TestBackupSelectedMods(t *testing.T) {
	source := instance.Instance{ID: 2, Loader: "Fabric", LoaderVersion: "0.16.14", Version: "1.21.1", GameDir: t.TempDir()}
	for _, name := range []string{"a.jar", "b.jar.disabled", "c.jar"} {
		writeTestMod(t, source, name, "fabric.mod.json", `{"id":"example","version":"1"}`)
	}
	files, err := content.Inventory(t.Context(), source, content.Mods)
	if err != nil || len(files) != 3 {
		t.Fatal(files, err)
	}
	chosen := files[:2]
	dir := t.TempDir()
	path := filepath.Join(dir, "backup.zip")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if err := content.BackupMods(ctx, source, chosen, path, func(p tasks.Progress) {
		if p.Done == 1 {
			cancel()
		}
	}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	changed := append([]content.File(nil), chosen...)
	changed[1].Size++
	if err := content.BackupMods(t.Context(), source, changed, path, nil); err == nil {
		t.Fatal("backed up a changed mod")
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatal("incomplete backup", entries, err)
	}
	if err := content.BackupMods(t.Context(), source, chosen, path, nil); err != nil {
		t.Fatal(err)
	}
	if err := content.BackupMods(t.Context(), source, files[2:], path, nil); !errors.Is(err, fs.ErrExist) {
		t.Fatal("reused existing target", err)
	}
	z, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	if len(z.File) != len(chosen) {
		t.Fatal("wrong selection", z.File)
	}
	for j, entry := range z.File {
		name := chosen[j].DiskName()
		if entry.Name != "mods/"+name {
			t.Fatal(entry.Name, name)
		}
		want, err := os.ReadFile(filepath.Join(source.GameDir, "mods", name))
		if err != nil {
			t.Fatal(err)
		}
		r, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(r)
		r.Close()
		if err != nil || !bytes.Equal(got, want) {
			t.Fatal(name, err)
		}
	}
}

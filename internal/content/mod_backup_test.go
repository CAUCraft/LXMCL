package content_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha512"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
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
	digests := make(map[string]string, len(chosen))
	for _, file := range chosen {
		data, err := os.ReadFile(filepath.Join(source.GameDir, "mods", file.DiskName()))
		if err != nil {
			t.Fatal(err)
		}
		digests[file.DiskName()] = fmt.Sprintf("%x", sha512.Sum512(data))
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "backup.zip")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if artifact, err := content.BackupVerifiedMods(ctx, source, chosen, digests, path, func(p tasks.Progress) {
		if p.Done == 1 {
			cancel()
		}
	}); !errors.Is(err, context.Canceled) || artifact != (tasks.LocalZIP{}) {
		t.Fatal(artifact, err)
	}
	changed := append([]content.File(nil), chosen...)
	changed[1].Size++
	if err := content.BackupMods(t.Context(), source, changed, path, nil); err == nil {
		t.Fatal("backed up a changed mod")
	}
	stale := chosen[1]
	name := stale.DiskName()
	sourcePath := filepath.Join(source.GameDir, "mods", name)
	writeTestMod(t, source, name, "fabric.mod.json", `{"id":"example","version":"2"}`)
	if err := os.Chtimes(sourcePath, stale.Modified, stale.Modified); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(sourcePath); err != nil || st.Size() != stale.Size || !st.ModTime().Equal(stale.Modified) {
		t.Fatal("expected unchanged size and timestamp", st, err)
	}
	if artifact, err := content.BackupVerifiedMods(t.Context(), source, chosen, digests, path, nil); err == nil || artifact != (tasks.LocalZIP{}) {
		t.Fatal("backed up different bytes with the same size and timestamp")
	}
	delete(digests, name)
	if artifact, err := content.BackupVerifiedMods(t.Context(), source, chosen, digests, path, nil); err == nil || artifact != (tasks.LocalZIP{}) {
		t.Fatal("backed up a mod without an expected digest")
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatal("incomplete backup", entries, err)
	}
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	digests[name] = fmt.Sprintf("%x", sha512.Sum512(data))
	artifact, err := content.BackupVerifiedMods(t.Context(), source, chosen, digests, path, nil)
	if err != nil || artifact.Path != path {
		t.Fatal(artifact, err)
	}
	row := (tasks.Record{InstanceID: source.ID, Action: tasks.ModUpdateAction, Status: tasks.Canceled, ModUpdateBackup: artifact}).Submitted()
	if runtime.GOOS == "linux" || runtime.GOOS == "windows" {
		if got, err := row.LocateModUpdateBackup(); err != nil || got != path {
			t.Fatal(got, err)
		}
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

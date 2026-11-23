package saves_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/CAUCraft/LXMCL/internal/saves"
)

func TestLocalBackupsListsManagedEntriesAndMissingRecords(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const id = 3
	dir := saves.BackupsDir(root, id)
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	const missing = "recorded-missing.zip"
	const older = "20260102-030405-ABC234.zip"
	known := []saves.BackupRecord{
		{Name: "recorded.ZIP", At: at.Add(3 * time.Hour)},
		{Name: missing, At: at},
		{Name: missing, At: at.Add(-time.Hour)},
		{Name: "../outside.zip", At: at},
		{Name: `outside\other.zip`, At: at},
		{Name: "not-a-zip.txt", At: at},
		{Name: "NUL.zip", At: at},
	}
	empty, err := saves.LocalBackups(t.Context(), root, id, known)
	if err != nil || len(empty) != 2 || empty[0].Name != "recorded.ZIP" || !empty[0].Missing || empty[1].Name != missing || !empty[1].Missing {
		t.Fatal("uncreated directory", empty, err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("created the backup directory", err)
	}
	contents := map[string]time.Time{
		"unrelated.zip":              at,
		"recorded.ZIP":               at.Add(-time.Hour),
		older:                        at.Add(-time.Hour),
		"20260230-030405-ABC234.zip": at,
		"20260105-030405-ABC189.zip": at,
		"20260106-030405-abc234.zip": at,
		"20260103-030405-ABC235.zip": at.Add(-2 * time.Hour),
		"20260104-030405-ABC236.zip": at.Add(-3 * time.Hour),
		"20260105-030405-ABC237.zip": at.Add(-4 * time.Hour),
	}
	if err := os.MkdirAll(filepath.Join(dir, "20260107-030405-ABC234.zip"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, mtime := range contents {
		path := filepath.Join(dir, name)
		// Listing recognizes metadata and the name rather than opening the ZIP.
		if err := os.WriteFile(path, []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
	got, err := saves.LocalBackups(t.Context(), root, id, known)
	want := []string{missing, older, "recorded.ZIP", "20260103-030405-ABC235.zip", "20260104-030405-ABC236.zip", "20260105-030405-ABC237.zip"}
	var names []string
	for _, entry := range got {
		names = append(names, entry.Name)
		if entry.Missing {
			if entry.Name != missing || !entry.Modified.Equal(at) || entry.Size != 0 {
				t.Errorf("missing record: %+v", entry)
			}
		} else if !entry.Modified.Equal(contents[entry.Name]) || entry.Size != int64(len(entry.Name)) {
			t.Errorf("file metadata: %+v", entry)
		}
	}
	if err != nil || !slices.Equal(names, want) {
		t.Fatal(names, err)
	}
	if got, err := saves.LocalBackups(t.Context(), root, id+1, nil); err != nil || len(got) != 0 {
		t.Fatal("another instance", got, err)
	}
	if checked, err := saves.CheckLocalBackup(root, id, older); err != nil || checked != filepath.Join(dir, older) {
		t.Fatal("existing backup", checked, err)
	}
	for _, name := range []string{"../outside.zip", "20260107-030405-ABC234.zip", missing} {
		if checked, err := saves.CheckLocalBackup(root, id, name); err == nil || checked != "" {
			t.Errorf("invalid or missing backup %q: %q %v", name, checked, err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := saves.LocalBackups(ctx, root, id, known); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled scan", err)
	}
}

func TestLocalBackupsIgnoresLinks(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := saves.BackupsDir(root, 2)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	name := "20260102-030405-ABC234.zip"
	outside := filepath.Join(root, name)
	if err := os.WriteFile(outside, []byte("outside the instance backup folder"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, name)); err != nil {
		t.Skipf("file symlinks unavailable: %v", err)
	}
	if rows, err := saves.LocalBackups(t.Context(), root, 2, []saves.BackupRecord{{Name: name}}); err != nil || len(rows) != 1 || !rows[0].Missing {
		t.Fatal("symlink was considered an available backup", rows, err)
	}
	if path, err := saves.CheckLocalBackup(root, 2, name); err == nil || path != "" {
		t.Fatal("symlink was accepted for preview", path, err)
	}
	if err := os.Symlink(dir, saves.BackupsDir(root, 4)); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}
	if rows, err := saves.LocalBackups(t.Context(), root, 4, nil); err == nil || len(rows) != 0 {
		t.Fatal("accepted another instance's linked directory", rows, err)
	}
	if path, err := saves.CheckLocalBackup(root, 4, name); err == nil || path != "" {
		t.Fatal("linked directory was accepted for preview", path, err)
	}
	other := filepath.Join(root, "other")
	if err := os.Mkdir(other, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(dir), filepath.Join(other, "backups")); err != nil {
		t.Fatal(err)
	}
	if rows, err := saves.LocalBackups(t.Context(), other, 2, nil); err == nil || len(rows) != 0 {
		t.Fatal("accepted a linked backups directory", rows, err)
	}
}

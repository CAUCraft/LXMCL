package saves_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/saves"
)

func TestExportLocalBackup(t *testing.T) {
	const id = 2
	name := "20260102-030405-ABC234.zip"
	firstRoot, secondRoot, destination := t.TempDir(), t.TempDir(), t.TempDir()
	first, second := backupZIP(t, "world one"), backupZIP(t, "world two")
	firstPath, entries := exportFixture(t, firstRoot, id, name, first)
	_, others := exportFixture(t, secondRoot, id, name, second)
	originalInfo, err := os.Stat(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(destination, "备份导出.zip")
	if err := saves.ExportLocalBackup(t.Context(), firstRoot, id, entries, path); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(first, got) {
		t.Fatal("exported bytes differ from selected archive", err)
	}

	if err := saves.ExportLocalBackup(t.Context(), secondRoot, id, entries, path); !errors.Is(err, saves.ErrLocalBackupChanged) {
		t.Fatal("reused a selection from another managed root", err)
	}
	if err := saves.ExportLocalBackup(t.Context(), secondRoot, id, others, path); !errors.Is(err, fs.ErrExist) {
		t.Fatal("existing destination", err)
	}
	got, err = os.ReadFile(path)
	if err != nil || !bytes.Equal(first, got) {
		t.Fatal("existing destination changed", err)
	}
	saved, err := os.ReadFile(firstPath)
	if err != nil || !bytes.Equal(first, saved) {
		t.Fatal("managed source changed", err)
	}
	info, err := os.Stat(firstPath)
	if err != nil || !os.SameFile(info, originalInfo) || !info.ModTime().Equal(originalInfo.ModTime()) {
		t.Fatal("managed source identity or time changed", err)
	}
	missingDestination := filepath.Join(destination, "absent", name)
	if err := saves.ExportLocalBackup(t.Context(), firstRoot, id, entries, missingDestination); !errors.Is(err, fs.ErrNotExist) || errors.Is(err, saves.ErrLocalBackupMissing) {
		t.Fatal("missing destination was reported as a missing source", err)
	}
	if err := saves.ExportLocalBackup(t.Context(), firstRoot, id, entries, "relative.zip"); err == nil {
		t.Fatal("relative destination was allowed")
	}
	if err := saves.ExportLocalBackup(t.Context(), firstRoot, id, entries, filepath.Join(destination, "invalid.txt")); err == nil {
		t.Fatal("non-ZIP destination was allowed")
	}

	if err := os.Rename(firstPath, firstPath+".old"); err != nil {
		t.Fatal(err)
	}
	newPath := filepath.Join(destination, "missing.zip")
	if err := saves.ExportLocalBackup(t.Context(), firstRoot, id, entries, newPath); !errors.Is(err, saves.ErrLocalBackupMissing) {
		t.Fatal("missing source was allowed", err)
	}
	if err := os.WriteFile(firstPath, second, 0600); err != nil {
		t.Fatal(err)
	}
	mtime := originalInfo.ModTime()
	if err := os.Chtimes(firstPath, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	if len(first) != len(second) {
		t.Fatal("replacement fixture must have equal size")
	}
	if err := saves.ExportLocalBackup(t.Context(), firstRoot, id, entries, newPath); !errors.Is(err, saves.ErrLocalBackupChanged) {
		t.Fatal("same-size replacement with the old modification time was allowed", err)
	}
	files, err := os.ReadDir(destination)
	if err != nil || len(files) != 1 || files[0].Name() != filepath.Base(path) {
		t.Fatal("failed export left unexpected files", files, err)
	}
}

func TestExportLocalBackupCancellationRemovesStagedFile(t *testing.T) {
	root, destination := t.TempDir(), t.TempDir()
	name := "20260102-030405-ABC234.zip"
	_, entry := exportFixture(t, root, 2, name, backupZIP(t, "world"))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	afterStage := &cancelBackupExport{Context: ctx, cancel: cancel, dir: destination}
	err := saves.ExportLocalBackup(afterStage, root, 2, entry, filepath.Join(destination, name))
	if !afterStage.staged || !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation after staging", afterStage.staged, err)
	}
	if files, err := os.ReadDir(destination); err != nil || len(files) != 0 {
		t.Fatal("cancellation left unexpected files", files, err)
	}
}

type cancelBackupExport struct {
	context.Context
	cancel context.CancelFunc
	dir    string
	staged bool
}

func (c *cancelBackupExport) Err() error {
	if err := c.Context.Err(); err != nil {
		return err
	}
	if entries, err := os.ReadDir(c.dir); err == nil {
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".part") {
				c.staged = true
				c.cancel()
				break
			}
		}
	}
	return c.Context.Err()
}

func backupZIP(t *testing.T, value string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	file, err := archive.CreateHeader(&zip.FileHeader{Name: "World/level.dat", Method: zip.Store})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte(value)); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func exportFixture(t *testing.T, root string, id int, name string, payload []byte) (string, saves.BackupEntry) {
	t.Helper()
	dir := saves.BackupsDir(root, id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, payload, 0600); err != nil {
		t.Fatal(err)
	}
	entries, err := saves.LocalBackups(t.Context(), root, id, nil)
	if err != nil || len(entries) != 1 || entries[0].Missing {
		t.Fatal("could not list export fixture", entries, err)
	}
	return path, entries[0]
}

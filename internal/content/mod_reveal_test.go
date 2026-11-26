package content_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

func TestModRevealPath(t *testing.T) {
	first := instance.Instance{ID: 1, Name: "同名实例", Loader: "Fabric", LoaderVersion: "0.16.14", Version: "1.21.1", GameDir: t.TempDir()}
	second := first
	second.ID, second.GameDir = 2, t.TempDir()
	const enabled, disabled = "地图.jar", "地图.jar.disabled"
	write := func(i instance.Instance, name string) string {
		t.Helper()
		dir := filepath.Join(i.GameDir, "mods")
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, name)
		// Unrecognized ordinary JAR files should still be locatable.
		if err := os.WriteFile(path, nil, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	write(first, disabled)
	activePath, disabledPath := write(second, enabled), write(second, disabled)

	files, err := content.Inventory(t.Context(), second, content.Mods)
	if err != nil || len(files) != 2 {
		t.Fatal(files, err)
	}
	byName := map[string]content.File{}
	for _, file := range files {
		byName[file.DiskName()] = file
	}
	for name, want := range map[string]string{enabled: activePath, disabled: disabledPath} {
		if got, err := byName[name].ModRevealPath(second); err != nil || got != want {
			t.Fatal(got, err, want)
		}
	}
	row := byName[disabled]
	if got, err := row.ModRevealPath(first); !errors.Is(err, fs.ErrInvalid) || got != "" {
		t.Fatal("revealed another instance's same-name file", got, err)
	}
	move := func(from, to string) {
		t.Helper()
		if err := os.Rename(from, to); err != nil {
			t.Fatal(err)
		}
	}
	move(disabledPath, filepath.Join(second.GameDir, "original"))
	if got, err := row.ModRevealPath(second); !errors.Is(err, fs.ErrNotExist) || got != "" {
		t.Fatal("missing disabled file fell back to the enabled version", got, err)
	}
	replacement := write(second, "replacement")
	if err := os.Chtimes(replacement, row.Modified, row.Modified); err != nil {
		t.Fatal(err)
	}
	move(replacement, disabledPath)
	if got, err := row.ModRevealPath(second); !errors.Is(err, fs.ErrInvalid) || got != "" {
		t.Fatal("revealed a same-name replacement with identical size and timestamp", got, err)
	}
}

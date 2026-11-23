package saves

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorldSelectionBacksUpOnlyTheChosenWorldAndRejectsReplacement(t *testing.T) {
	game := t.TempDir()
	ctx := t.Context()
	worlds, err := Worlds(ctx, game)
	if err != nil || len(worlds) != 0 {
		t.Fatal(worlds, err)
	}
	base := filepath.Join(game, "saves")
	if _, err = os.Stat(base); !os.IsNotExist(err) {
		t.Fatal("created saves", err)
	}
	makeWorld := func(name string) {
		t.Helper()
		dir := filepath.Join(base, name)
		if e := os.MkdirAll(dir, 0700); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(filepath.Join(dir, "level.dat"), []byte("world"), 0600); e != nil {
			t.Fatal(e)
		}
	}
	makeWorld("First")
	makeWorld("Second")
	worlds, err = Worlds(ctx, game)
	if err != nil || len(worlds) != 2 || worlds[0].Folder() != "First" || worlds[1].Folder() != "Second" {
		t.Fatal(worlds, err)
	}
	if err := worlds[0].Check(game); err != nil {
		t.Fatal(err)
	}
	dataRoot := t.TempDir()
	archive, err := BackupWorld(ctx, game, dataRoot, 1, worlds[0])
	if err != nil {
		t.Fatal(err)
	}
	preview, err := Inspect(ctx, archive)
	if err != nil {
		t.Fatal(err)
	}
	names := preview.Worlds()
	if len(names) != 1 || names[0] != "First" {
		t.Fatal(names)
	}
	if err := os.Rename(filepath.Join(base, "First"), filepath.Join(base, "Moved")); err != nil {
		t.Fatal(err)
	}
	makeWorld("First")
	if worlds[0].Check(game) == nil {
		t.Fatal("accepted replacement at old path")
	}
	if _, err := BackupWorld(ctx, game, dataRoot, 1, worlds[0]); err == nil {
		t.Fatal("archived a replacement world")
	}
	if err := worlds[1].Check(game); err != nil {
		t.Fatal(err)
	}
}

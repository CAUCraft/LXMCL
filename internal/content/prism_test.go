package content

import (
	"archive/zip"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/CAUCraft/LXMCL/internal/tasks"
)

func TestPrismImportLimitsAndRechecksPreview(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	pack := func(component string) string {
		return `{"formatVersion":1,"components":[{"uid":"net.minecraft","version":"1.21.4"},{"uid":"` + component + `","version":"3.3.3"}]}`
	}
	write("instance.cfg", "[General]\nname=Island\nOverrideMemory=true\nMaxMemAlloc=3072\nOverrideWindow=false\nMinecraftWinWidth=1600\n")
	write("mmc-pack.json", pack("org.lwjgl3"))
	write("minecraft/saves/Island/level.dat", "sample world")
	write("minecraft/saves/notes.txt", "not a folder")
	if err := os.Mkdir(filepath.Join(dir, "minecraft", "saves", "空目录"), 0700); err != nil {
		t.Fatal(err)
	}
	write("minecraft/options.txt", "fov:0.5")
	write("minecraft/servers.dat", "synthetic list bytes")
	write("minecraft/servers.dat_old", "previous list")
	write("minecraft/mods/marker", "not selected")
	p, err := ReadPrismInstance(t.Context(), dir)
	if err != nil || !p.Worlds || !p.Options || !p.Servers || p.Name != "Island" || p.Version != "1.21.4" || p.Launch.Memory != 3072 || p.Launch.Width != 0 {
		t.Fatal(p, err)
	}
	assertPrismWorldFolders(t, p, "Island", "空目录")
	dst := filepath.Join(t.TempDir(), "copy")
	commit := func() error { return nil }
	if err := CopyPrismInstance(t.Context(), p, dst, true, false, commit, nil); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dst)
	if err != nil || len(entries) != 1 || entries[0].Name() != "saves" {
		t.Fatal(entries, err)
	}
	if b, e := os.ReadFile(filepath.Join(dst, "saves", "Island", "level.dat")); e != nil || string(b) != "sample world" {
		t.Fatal(string(b), e)
	}
	if b, e := os.ReadFile(filepath.Join(dst, "saves", "notes.txt")); e != nil || string(b) != "not a folder" {
		t.Fatal(string(b), e)
	}
	if err := CopyPrismInstance(t.Context(), p, dst, false, true, commit, nil); err == nil {
		t.Fatal("occupied target accepted")
	}
	selected := PrismFiles{Servers: true}
	serverOutput := filepath.Join(t.TempDir(), "servers")
	if err := CopyPrismFiles(t.Context(), p, serverOutput, selected, commit, nil); err != nil {
		t.Fatal(err)
	}
	assertPrismServerList(t, serverOutput, "synthetic list bytes")
	lostOutput := filepath.Join(t.TempDir(), "lost-server")
	err = CopyPrismFiles(t.Context(), p, lostOutput, selected, func() error {
		t.Fatal("partial import attempted to commit")
		return nil
	}, func(tasks.Progress) {
		if err := os.Remove(filepath.Join(dir, "minecraft", "servers.dat")); err != nil {
			t.Fatal(err)
		}
	})
	if err == nil {
		t.Fatal("accepted a selected list that disappeared during staging")
	}
	if entries, err := os.ReadDir(filepath.Dir(lostOutput)); err != nil || len(entries) != 0 {
		t.Fatal(entries, err)
	}
	if err := CopyPrismFiles(t.Context(), p, lostOutput, PrismFiles{}, commit, nil); err != nil {
		t.Fatalf("change to an unselected directory list invalidated import: %v", err)
	}
	for _, name := range []string{"Island", "空目录"} {
		if err := os.RemoveAll(filepath.Join(dir, "minecraft", "saves", name)); err != nil {
			t.Fatal(err)
		}
	}
	if filesOnly, err := ReadPrismInstance(t.Context(), dir); err != nil || !filesOnly.Worlds {
		t.Fatal(filesOnly, err)
	} else {
		assertPrismWorldFolders(t, filesOnly)
	}
	write("instance.cfg", "name=Different")
	changed := filepath.Join(t.TempDir(), "changed")
	if err := CopyPrismInstance(t.Context(), p, changed, true, true, commit, nil); err == nil {
		t.Fatal("stale preview accepted")
	}
	if _, err := os.Stat(changed); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	for _, uid := range []string{"net.fabricmc.fabric-loader", "example.extra"} {
		write("mmc-pack.json", pack(uid))
		if _, err := ReadPrismInstance(t.Context(), dir); err == nil || !strings.Contains(err.Error(), uid) {
			t.Fatal(uid, err)
		}
	}
}

func TestPrismZIPRechecksAndStagesSelectedFiles(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "Island.zip")
	write := func(world, servers string) {
		t.Helper()
		f, err := os.Create(filename)
		if err != nil {
			t.Fatal(err)
		}
		z := zip.NewWriter(f)
		files := fstest.MapFS{
			"instance.cfg":                      {Data: []byte("name=Island\n")},
			"mmc-pack.json":                     {Data: []byte(`{"formatVersion":1,"components":[{"uid":"net.minecraft","version":"1.21.4"}]}`)},
			".minecraft/saves/Island/level.dat": {Data: []byte(world)},
			".minecraft/saves/Empty":            {Mode: fs.ModeDir},
			".minecraft/saves/notes.txt":        {Data: []byte("not a folder")},
			".minecraft/options.txt":            {Data: []byte("music:0.25")},
			".minecraft/servers.dat":            {Data: []byte(servers)},
			".minecraft/servers.dat_old":        {Data: []byte("previous list")},
			".minecraft/mods/notes.txt":         {Data: []byte("not selected")},
		}
		if err := z.AddFS(files); err != nil {
			t.Fatal(err)
		}
		// A ZIP may have nested files without explicit directory entries.
		if _, err := z.Create(".minecraft/saves/隐式/region/ordinary.dat"); err != nil {
			t.Fatal(err)
		}
		if err := errors.Join(z.Close(), f.Close()); err != nil {
			t.Fatal(err)
		}
	}
	write("first", "archive server list")
	ctx := t.Context()
	p, err := ReadPrismZIP(ctx, filename)
	if err != nil || !p.Worlds || !p.Options || !p.Servers || p.Version != "1.21.4" {
		t.Fatal(p, err)
	}
	assertPrismWorldFolders(t, p, "Empty", "Island", "隐式")
	parent := t.TempDir()
	dst := filepath.Join(parent, "new")
	commit := func() error { return nil }
	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	err = CopyPrismInstance(cancelCtx, p, dst, true, true, commit, func(progress tasks.Progress) {
		if progress.Done == 1 {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	write("later", "archive server list")
	if err := CopyPrismInstance(ctx, p, dst, true, false, commit, nil); err == nil {
		t.Fatal("stale world accepted")
	}
	if entries, err := os.ReadDir(parent); err != nil || len(entries) != 0 {
		t.Fatal(entries, err)
	}
	p, err = ReadPrismZIP(ctx, filename)
	if err != nil {
		t.Fatal(err)
	}
	if err := CopyPrismInstance(ctx, p, dst, false, true, commit, nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dst, "options.txt"))
	if err != nil || string(data) != "music:0.25" {
		t.Fatal(string(data), err)
	}
	if entries, err := os.ReadDir(dst); err != nil || len(entries) != 1 {
		t.Fatal(entries, err)
	}
	serverOutput := filepath.Join(parent, "servers")
	if err := CopyPrismFiles(ctx, p, serverOutput, PrismFiles{Servers: true}, commit, nil); err != nil {
		t.Fatal(err)
	}
	assertPrismServerList(t, serverOutput, "archive server list")
	write("later", "edited server list")
	changed := filepath.Join(parent, "changed")
	if err := CopyPrismFiles(ctx, p, changed, PrismFiles{}, commit, nil); err == nil {
		t.Fatal("a ZIP changed after preview must require a new preview")
	}
	if _, err := os.Stat(changed); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}

func assertPrismServerList(t *testing.T, dir, want string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "servers.dat" {
		t.Fatal(entries, err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "servers.dat"))
	if err != nil || string(data) != want {
		t.Fatal(string(data), err)
	}
}

func assertPrismWorldFolders(t *testing.T, preview PrismInstance, want ...string) {
	t.Helper()
	got, complete := preview.WorldFolders()
	if !complete || !slices.Equal(got, want) {
		t.Fatalf("WorldFolders = %q, %v, want %q and a complete list", got, complete, want)
	}
}

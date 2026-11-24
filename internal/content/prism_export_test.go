package content

import (
	"archive/zip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/tasks"
)

func TestPrismExportRoundTrip(t *testing.T) {
	ctx := t.Context()
	i := instance.Instance{Name: "建筑 #2\\园区", Version: "1.21.4", GameDir: t.TempDir(), Launch: instance.LaunchOptions{Memory: 3584, Display: instance.DisplayResolution, Width: 1280, Height: 720, JavaMode: instance.JavaCustom, JavaPath: filepath.Join(t.TempDir(), "java")}}
	for _, p := range []string{"options.txt", "servers.dat", "servers.dat_old", "saves/Island/level.dat", "saves/Island/region/r.0.0.mca", "accounts.json", "scripts/start.txt", "saves/Island/session.lock", "saves/Island/notes.txt", "saves/Island/datapacks/demo/data/demo/function/init.mcfunction"} {
		file := filepath.Join(i.GameDir, p)
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(p), 0600); err != nil {
			t.Fatal(err)
		}
	}
	out := t.TempDir()
	filename := filepath.Join(out, "carry.zip")
	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := ExportPrismZIP(cancelCtx, i, filename, true, true, func(p tasks.Progress) {
		if p.Done == 2 {
			cancel()
		}
	}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if files, err := os.ReadDir(out); err != nil || len(files) != 0 {
		t.Fatal(files, err)
	}
	// options.txt is staged before world data. A source read failure must discard both.
	worldDir := filepath.Join(i.GameDir, "saves")
	held := filepath.Join(t.TempDir(), "worlds")
	if err := os.Rename(worldDir, held); err != nil {
		t.Fatal(err)
	}
	if err := ExportPrismZIP(ctx, i, filename, true, true, nil); err == nil {
		t.Fatal("missing selected worlds")
	}
	if entries, err := os.ReadDir(out); err != nil || len(entries) != 0 {
		t.Fatal(entries, err)
	}
	if err := os.Rename(held, worldDir); err != nil {
		t.Fatal(err)
	}
	if err := ExportPrismZIP(ctx, i, filepath.Join(i.GameDir, "inside.zip"), false, false, nil); err == nil {
		t.Fatal("published inside source")
	}
	for _, tc := range []struct {
		name  string
		files PrismFiles
	}{
		{"config", PrismFiles{}},
		{"options", PrismFiles{Options: true}},
		{"worlds", PrismFiles{Worlds: true}},
		{"both", PrismFiles{Worlds: true, Options: true}},
		{"servers", PrismFiles{Servers: true}},
		{"all", PrismFiles{Worlds: true, Options: true, Servers: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			filename := filepath.Join(out, tc.name+".zip")
			dst := filepath.Join(t.TempDir(), "new")
			if err := ExportPrismFiles(ctx, i, filename, tc.files, nil); err != nil {
				t.Fatal(err)
			}
			preview, err := ReadPrismZIP(ctx, filename)
			want := instance.LaunchOptions{Display: instance.DisplayResolution, Width: 1280, Height: 720, Memory: 3584}
			if err != nil || preview.Name != i.Name || preview.Version != i.Version || preview.Launch != want || preview.Options != tc.files.Options || preview.Worlds != tc.files.Worlds || preview.Servers != tc.files.Servers {
				t.Fatal(preview, err)
			}
			if err := CopyPrismFiles(ctx, preview, dst, tc.files, func() error { return nil }, nil); err != nil {
				t.Fatal(err)
			}
			files := []string{"instance/instance.cfg", "instance/mmc-pack.json"}
			check := func(name string) {
				b, e := os.ReadFile(filepath.Join(dst, name))
				if e != nil || string(b) != name {
					t.Fatal(name, e)
				}
				files = append(files, "instance/.minecraft/"+name)
			}
			if tc.files.Options {
				check("options.txt")
			}
			if tc.files.Servers {
				check("servers.dat")
			}
			if tc.files.Worlds {
				check("saves/Island/level.dat")
				check("saves/Island/region/r.0.0.mca")
			}
			z, e := zip.OpenReader(filename)
			if e != nil {
				t.Fatal(e)
			}
			var names []string
			for _, f := range z.File {
				names = append(names, f.Name)
			}
			if err := z.Close(); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(files, names) {
				t.Fatal(names)
			}
		})
	}
}

func TestPrismExportDoesNotPublishMissingServerList(t *testing.T) {
	ctx := t.Context()
	i := instance.Instance{Name: "Island", Version: "1.21.4", GameDir: t.TempDir()}
	source := filepath.Join(i.GameDir, serverListPath.name)
	out := t.TempDir()
	filename := filepath.Join(out, "carry.zip")
	selected := PrismFiles{Servers: true}
	if err := ExportPrismFiles(ctx, i, filename, selected, nil); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected an error for a missing selected list, got %v", err)
	}
	if err := os.WriteFile(source, []byte("sample server list"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ExportPrismFiles(ctx, i, filename, selected, func(tasks.Progress) {
		if err := os.Remove(source); err != nil {
			t.Fatal(err)
		}
	}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected an error for a removed selected list, got %v", err)
	}
	if entries, err := os.ReadDir(out); err != nil || len(entries) != 0 {
		t.Fatal(entries, err)
	}
	if err := ExportPrismFiles(ctx, i, filename, PrismFiles{}, nil); err != nil {
		t.Fatalf("default export depends on the unselected list: %v", err)
	}
}

func TestPrismExportSelectedWorlds(t *testing.T) {
	ctx := t.Context()
	i := instance.Instance{Name: "Worlds", Version: "1.21.4", GameDir: t.TempDir()}
	write := func(name string) {
		t.Helper()
		filename := filepath.Join(i.GameDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{
		"saves/Island/level.dat", "saves/Island/region/r.0.0.mca", "saves/Island/notes.txt",
		"saves/Island Annex/level.dat", "saves/Not a world/notes.txt",
		"options.txt", "servers.dat",
	} {
		write(name)
	}
	preview, err := PreviewPrismExportWorlds(ctx, i)
	if err != nil {
		t.Fatal(err)
	}
	if got := preview.WorldFolders(); !slices.Equal(got, []string{"Island", "Island Annex"}) {
		t.Fatal(got)
	}
	selected := []string{"Island"}
	files := PrismFiles{Worlds: true, Options: true, Servers: true}
	write("saves/Island Annex/region/r.1.1.mca")
	filename := filepath.Join(t.TempDir(), "selected.zip")
	if err := ExportPrismSelectedWorlds(ctx, i, filename, files, preview, selected, nil); err != nil {
		t.Fatal(err)
	}
	z, err := zip.OpenReader(filename)
	if err != nil {
		t.Fatal(err)
	}
	var members []string
	for _, entry := range z.File {
		members = append(members, entry.Name)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if want := []string{"instance/instance.cfg", "instance/mmc-pack.json", "instance/.minecraft/options.txt",
		"instance/.minecraft/servers.dat", "instance/.minecraft/saves/Island/level.dat", "instance/.minecraft/saves/Island/region/r.0.0.mca"}; !slices.Equal(members, want) {
		t.Fatalf("ZIP members = %q, want %q", members, want)
	}
	imported, err := ReadPrismZIP(ctx, filename)
	if err != nil {
		t.Fatal(err)
	}
	assertPrismWorldFolders(t, imported, "Island")
	dst := filepath.Join(t.TempDir(), "reimport")
	importFiles := files
	importFiles.WorldFolders = selected
	if err := CopyPrismFiles(ctx, imported, dst, importFiles, func() error { return nil }, nil); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(dst, "saves", "Island", "region", "r.0.0.mca")); err != nil || string(b) != "saves/Island/region/r.0.0.mca" {
		t.Fatal(string(b), err)
	}

	failureDir := t.TempDir()
	failed := filepath.Join(failureDir, "failed.zip")
	for _, bad := range [][]string{{}, {"ISLAND"}, {"Island", "Island"}} {
		if err := ExportPrismSelectedWorlds(ctx, i, failed, files, preview, bad, nil); !errors.Is(err, ErrPrismExportWorldsChanged) {
			t.Fatalf("invalid selection %q: %v", bad, err)
		}
	}
	importSelection := PrismFiles{Worlds: true, WorldFolders: []string{"Island"}}
	if err := ExportPrismFiles(ctx, i, failed, importSelection, nil); !errors.Is(err, ErrPrismExportWorldsChanged) {
		t.Fatalf("legacy export accepted import-only selection: %v", err)
	}
	if err := ExportPrismSelectedWorlds(ctx, i, failed, importSelection, preview, selected, nil); !errors.Is(err, ErrPrismExportWorldsChanged) {
		t.Fatalf("selected export accepted import-only selection: %v", err)
	}
	worldsOnly := PrismFiles{Worlds: true}
	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := ExportPrismSelectedWorlds(cancelCtx, i, failed, worldsOnly, preview, selected, func(tasks.Progress) {
		cancel()
	}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := ExportPrismSelectedWorlds(ctx, i, failed, worldsOnly, preview, selected, func(tasks.Progress) {
		write("saves/Island/region/r.2.2.mca")
	}); !errors.Is(err, ErrPrismExportWorldsChanged) {
		t.Fatalf("changed selected files accepted: %v", err)
	}
	preview, err = PreviewPrismExportWorlds(ctx, i)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(i.GameDir, "saves", "Island"), filepath.Join(t.TempDir(), "old")); err != nil {
		t.Fatal(err)
	}
	write("saves/Island/level.dat")
	if err := ExportPrismSelectedWorlds(ctx, i, failed, worldsOnly, preview, selected, nil); !errors.Is(err, ErrPrismExportWorldsChanged) {
		t.Fatalf("same-name replacement accepted: %v", err)
	}
	if entries, err := os.ReadDir(failureDir); err != nil || len(entries) != 0 {
		t.Fatal(entries, err)
	}
}

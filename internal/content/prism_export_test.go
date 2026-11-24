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

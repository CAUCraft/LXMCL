package content

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	write("minecraft/options.txt", "fov:0.5")
	write("minecraft/mods/marker", "not selected")
	p, err := ReadPrismInstance(t.Context(), dir)
	if err != nil || !p.Worlds || !p.Options || p.Name != "Island" || p.Version != "1.21.4" || p.Launch.Memory != 3072 || p.Launch.Width != 0 {
		t.Fatal(p, err)
	}
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
	if err := CopyPrismInstance(t.Context(), p, dst, false, true, commit, nil); err == nil {
		t.Fatal("occupied target accepted")
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

package content_test

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

func writeTestMod(t *testing.T, i instance.Instance, file, marker, manifest string) {
	t.Helper()
	dir := filepath.Join(i.GameDir, "mods")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, file))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z := zip.NewWriter(f)
	w, err := z.Create(marker)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.WriteString(w, manifest); err != nil {
		t.Fatal(err)
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCompareInstalledMods(t *testing.T) {
	base := instance.Instance{ID: 1, Name: "Base", Loader: "Fabric", Version: "1.21.1", GameDir: t.TempDir()}
	other := instance.Instance{ID: 2, Name: "Other", Loader: "Forge", Version: "1.21.1", GameDir: t.TempDir()}
	fabric := func(file, id, version string) {
		writeTestMod(t, base, file, "fabric.mod.json", fmt.Sprintf(`{"id":%q,"name":"Twin","version":%q}`, id, version))
	}
	fabric("old-core.jar", "core", "1")
	fabric("look-alike.jar", "before", "1")
	fabric("renamed.jar", "stable", "1")
	fabric("torch-old.jar", "torch", "1.2")
	fabric("a.jar", "ambiguous", "1")
	fabric("b.jar", "ambiguous", "2")
	writeTestMod(t, base, "plain.jar", "notice.txt", "local fixture")
	writeTestMod(t, other, "bundle.jar.disabled", "META-INF/mods.toml", "[[mods]]\nmodId='core'\nversion='2'\n[[mods]]\nmodId='after'\nversion='1'")
	writeTestMod(t, other, "look-alike.jar", "META-INF/mods.toml", "[[mods]]\nmodId='stable'\nversion='1'")
	writeTestMod(t, other, "plain.jar.disabled", "notice.txt", "local fixture")
	writeTestMod(t, other, "torch-new.jar", "META-INF/mods.toml", "[[mods]]\nmodId='torch'\nversion='1.3'")
	got, err := content.CompareMods(t.Context(), base, other)
	if err != nil {
		t.Fatal(err)
	}
	want := content.ModComparison{Rows: []content.ModDifference{
		{Name: "after", After: &content.ModState{Version: "1", Enabled: false}},
		{Name: "before", Before: &content.ModState{Version: "1", Enabled: true}},
		{Name: "core", Before: &content.ModState{Version: "1", Enabled: true}, After: &content.ModState{Version: "2", Enabled: false}},
		{Name: "plain.jar", ByFilename: true, Before: &content.ModState{Enabled: true}, After: &content.ModState{Enabled: false}},
		{Name: "torch", Before: &content.ModState{Version: "1.2", Enabled: true}, After: &content.ModState{Version: "1.3", Enabled: true}},
	}, Ambiguous: []string{"ambiguous"}, UnknownVersions: 1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected diff: %+v", got)
	}
	if !got.Rows[2].VersionChanged() || got.Rows[3].VersionChanged() {
		t.Fatal("unknown version interpreted as comparable")
	}
	empty := other
	empty.GameDir = t.TempDir()
	if _, err := content.CompareMods(t.Context(), empty, base); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(empty.GameDir, "mods")); !os.IsNotExist(err) {
		t.Fatal("read-only scan created mods directory", err)
	}
}

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

func TestSavedResourcePackOrder(t *testing.T) {
	i := instance.Instance{GameDir: t.TempDir()}
	packDir := filepath.Join(i.GameDir, "resourcepacks")
	if err := os.MkdirAll(filepath.Join(packDir, "vanilla"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"vanilla/pack.mcmeta", "秋.zip", "秋.zip.disabled", "旧.zip.disabled"} {
		if err := os.WriteFile(filepath.Join(packDir, n), []byte("{}"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	options := filepath.Join(i.GameDir, "options.txt")
	read := func(s string) ([]content.SavedPack, error) {
		t.Helper()
		if err := os.WriteFile(options, []byte(s), 0644); err != nil {
			t.Fatal(err)
		}
		return content.SavedResourcePacks(t.Context(), i)
	}
	if _, err := content.SavedResourcePacks(t.Context(), i); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	rows, err := read("incompatibleResourcePacks:[\"file/no.zip\"]\r\n")
	if err != nil || rows != nil {
		t.Fatal(rows, err)
	}
	rows, err = read("resourcePacks:[]")
	if err != nil || rows == nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
	for _, v := range []string{"", "null", "[null]", "[1]", "[\"a\""} {
		if rows, err := read("resourcePacks:" + v); err == nil || rows != nil {
			t.Fatal(v, rows, err)
		}
	}
	rows, err = read("\ufeffresourcePacks:[\"old\"]\r\n" + `resourcePacks:["file/秋.zip","vanilla","file/vanilla","旧.zip","file/无.zip","mod:other","programer_art","file/秋.zip","fabric"]`)
	want := []content.SavedPackKind{content.LocalPack, content.BuiltinPack, content.LocalPack, content.LocalPack, content.MissingPack, content.OtherPackID, content.BuiltinPack, content.LocalPack, content.LoaderPackID}
	if err != nil || len(rows) != len(want) {
		t.Fatal(rows, err)
	}
	for j, k := range want {
		if rows[j].Kind != k || (rows[j].File != nil) != (k == content.LocalPack) {
			t.Fatal(j, rows[j])
		}
	}
	if !rows[0].File.Enabled || rows[0].File.Name != "秋.zip" || !rows[2].File.Directory || rows[3].File.DiskName() != "旧.zip.disabled" || rows[5].ID != "mod:other" || rows[0].ID != rows[7].ID {
		t.Fatal(rows)
	}
}

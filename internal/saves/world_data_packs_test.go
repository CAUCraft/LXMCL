package saves_test

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/saves"
)

func TestReadWorldDataPacks(t *testing.T) {
	game := t.TempDir()
	enabled := []string{"vanilla", "file/汉字目录", "file/资源.ZIP", "mod:bundle", "file/不见的.zip", "file/说明.txt"}
	disabled := []string{"file/未完成", "custom:disabled"}
	data := fixtureDataPacks(t, enabled, disabled)
	worldDir := writeDataPackFile(t, game, data)
	packDir := filepath.Join(worldDir, "datapacks")
	for _, name := range []string{"汉字目录", "未完成"} {
		if err := os.MkdirAll(filepath.Join(packDir, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"资源.ZIP", "汉字目录/pack.mcmeta", "说明.txt"} {
		if err := os.WriteFile(filepath.Join(packDir, name), []byte("only the file type is listed"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	world := dataPackWorld(t, game)
	got, err := saves.ReadWorldDataPacks(t.Context(), game, world)
	if err != nil || !got.Saved || got.SavedErr != nil {
		t.Fatal(got, err)
	}
	if !slices.Equal(got.Local, []saves.DataPack{
		{Name: "未完成", Kind: saves.DataPackUnsupported},
		{Name: "汉字目录", Kind: saves.DataPackFolder},
		{Name: "说明.txt", Kind: saves.DataPackUnsupported},
		{Name: "资源.ZIP", Kind: saves.DataPackZIP},
	}) {
		t.Fatal("local entries", got.Local)
	}
	wantEnabled := []saves.DataPackKind{saves.DataPackNonFile, saves.DataPackFolder, saves.DataPackZIP, saves.DataPackNonFile, saves.DataPackMissing, saves.DataPackUnsupported}
	for _, group := range []struct {
		packs []saves.DataPack
		names []string
		kinds []saves.DataPackKind
	}{
		{got.Enabled, enabled, wantEnabled},
		{got.Disabled, disabled, []saves.DataPackKind{saves.DataPackUnsupported, saves.DataPackNonFile}},
	} {
		if len(group.packs) != len(group.names) {
			t.Fatal(group.packs)
		}
		for i, entry := range group.packs {
			if entry.Name != group.names[i] || entry.Kind != group.kinds[i] {
				t.Fatal("saved order, spelling or kind", group.packs)
			}
		}
	}
	if after, err := os.ReadFile(filepath.Join(worldDir, "level.dat")); err != nil || !bytes.Equal(after, data) {
		t.Fatal("reading the view changed the save", err)
	}

	other := t.TempDir()
	otherDir := writeDataPackFile(t, other, []byte("unreadable level.dat"))
	if err := os.Mkdir(filepath.Join(otherDir, "datapacks"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(otherDir, "datapacks", "other.zip"), []byte("other"), 0600); err != nil {
		t.Fatal(err)
	}
	if crossed, err := saves.ReadWorldDataPacks(t.Context(), other, world); err == nil || len(crossed.Local) != 0 {
		t.Fatal("a same-name world in another instance was opened", crossed, err)
	}
	corrupt, err := saves.ReadWorldDataPacks(t.Context(), other, dataPackWorld(t, other))
	if err != nil || corrupt.SavedErr == nil || corrupt.Saved || len(corrupt.Enabled) != 0 || len(corrupt.Disabled) != 0 ||
		!slices.Equal(corrupt.Local, []saves.DataPack{{Name: "other.zip", Kind: saves.DataPackZIP}}) {
		t.Fatal("corrupt level.dat should not hide local files", corrupt, err)
	}
}

func TestWorldDataPacksMissingDirectoryAndSavedLists(t *testing.T) {
	for _, tc := range []struct {
		label     string
		enabled   []string
		disabled  []string
		hasRecord bool
	}{
		{label: "absent"},
		{label: "empty", enabled: []string{}, disabled: []string{}, hasRecord: true},
		{label: "missing file", enabled: []string{"file/旧资源.zip"}, disabled: []string{}, hasRecord: true},
	} {
		t.Run(tc.label, func(t *testing.T) {
			game := t.TempDir()
			dir := writeDataPackFile(t, game, fixtureDataPacks(t, tc.enabled, tc.disabled))
			packs, err := saves.ReadWorldDataPacks(t.Context(), game, dataPackWorld(t, game))
			if err != nil || packs.SavedErr != nil || packs.Saved != tc.hasRecord || len(packs.Local) != 0 || len(packs.Enabled) != len(tc.enabled) {
				t.Fatal(packs, err)
			}
			if len(tc.enabled) == 1 && packs.Enabled[0] != (saves.DataPack{Name: tc.enabled[0], Kind: saves.DataPackMissing}) {
				t.Fatal(packs.Enabled)
			}
			if _, err := os.Stat(filepath.Join(dir, "datapacks")); !errors.Is(err, fs.ErrNotExist) {
				t.Fatal("created a datapacks folder", err)
			}
		})
	}
}

func fixtureDataPacks(t *testing.T, enabled, disabled []string) []byte {
	t.Helper()
	var b bytes.Buffer
	str := func(s string) {
		b.Write(binary.BigEndian.AppendUint16(nil, uint16(len(s))))
		b.WriteString(s)
	}
	list := func(name string, values []string) {
		b.WriteByte(9)
		str(name)
		if len(values) == 0 {
			b.WriteByte(0)
		} else {
			b.WriteByte(8)
		}
		b.Write(binary.BigEndian.AppendUint32(nil, uint32(len(values))))
		for _, value := range values {
			str(value)
		}
	}
	b.WriteByte(10)
	str("")
	b.WriteByte(10)
	str("Data")
	if enabled != nil || disabled != nil {
		b.WriteByte(10)
		str("DataPacks")
		list("Enabled", enabled)
		list("Disabled", disabled)
		b.WriteByte(0)
	}
	b.Write([]byte{0, 0})
	var contents bytes.Buffer
	g := gzip.NewWriter(&contents)
	if _, err := g.Write(b.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	return contents.Bytes()
}

func writeDataPackFile(t *testing.T, game string, contents []byte) string {
	t.Helper()
	dir := filepath.Join(game, "saves", "同名世界")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "level.dat"), contents, 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func dataPackWorld(t *testing.T, game string) saves.World {
	t.Helper()
	worlds, err := saves.Worlds(t.Context(), game)
	if err != nil || len(worlds) != 1 {
		t.Fatal(worlds, err)
	}
	return worlds[0]
}

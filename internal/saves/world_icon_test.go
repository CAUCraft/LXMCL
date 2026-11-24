package saves_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/iconimage"
	"github.com/CAUCraft/LXMCL/internal/saves"
)

func TestWorldIconReadsOnlySelectedSnapshot(t *testing.T) {
	firstDir, secondDir := t.TempDir(), t.TempDir()
	first, firstPath := createWorldIconFixture(t, firstDir)
	second, secondPath := createWorldIconFixture(t, secondDir)
	ctx := t.Context()
	if got, err := first.Icon(ctx, firstDir); !errors.Is(err, fs.ErrNotExist) || got != nil {
		t.Fatal("missing icon", got, err)
	}
	red, blue := color.RGBA{R: 220, A: 255}, color.RGBA{B: 220, A: 255}
	data := writeWorldIcon(t, firstPath, red)
	writeWorldIcon(t, secondPath, blue)
	for _, tc := range []struct {
		world saves.World
		dir   string
		want  color.RGBA
	}{
		{first, firstDir, red}, {second, secondDir, blue},
	} {
		icon, err := tc.world.Icon(ctx, tc.dir)
		if err != nil || icon == nil {
			t.Fatal("read a world icon", err)
		}
		b := icon.Bounds()
		if b.Dx() != 96 || b.Dy() != 96 || icon.At(48, 48) != tc.want {
			t.Fatal("wrong decoded icon", b, icon.At(48, 48))
		}
	}
	if icon, err := first.Icon(ctx, secondDir); err == nil || icon != nil {
		t.Fatal("opened the other instance's same-name world", err)
	}
	if contents, err := os.ReadFile(firstPath); err != nil || !bytes.Equal(contents, data) {
		t.Fatal("icon data changed", err)
	}
	if err := os.WriteFile(firstPath, []byte("corrupt PNG"), 0600); err != nil {
		t.Fatal(err)
	}
	if icon, err := first.Icon(ctx, firstDir); err == nil || icon != nil {
		t.Fatal("accepted a corrupt icon", err)
	}
	if err := os.Truncate(firstPath, iconimage.MaxPreviewBytes+1); err != nil {
		t.Fatal(err)
	}
	if icon, err := first.Icon(ctx, firstDir); err == nil || icon != nil {
		t.Fatal("accepted an oversized icon", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if icon, err := second.Icon(canceled, secondDir); !errors.Is(err, context.Canceled) || icon != nil {
		t.Fatal("canceled icon read succeeded", err)
	}
}

func TestWorldIconRejectsLinkedFile(t *testing.T) {
	gameDir := t.TempDir()
	world, filename := createWorldIconFixture(t, gameDir)
	other := filepath.Join(t.TempDir(), "other.png")
	writeWorldIcon(t, other, color.RGBA{B: 220, A: 255})
	if err := os.Symlink(other, filename); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	if icon, err := world.Icon(t.Context(), gameDir); err == nil || icon != nil {
		t.Fatal("accepted a linked icon", err)
	}
}

func createWorldIconFixture(t *testing.T, gameDir string) (saves.World, string) {
	t.Helper()
	world := filepath.Join(gameDir, "saves", "河谷")
	if err := os.MkdirAll(world, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(world, "level.dat"), []byte("local world fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	worlds, err := saves.Worlds(t.Context(), gameDir)
	if err != nil || len(worlds) != 1 {
		t.Fatal(worlds, err)
	}
	return worlds[0], filepath.Join(world, "icon.png")
}

func writeWorldIcon(t *testing.T, filename string, fill color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 2))
	for i := 0; i < 4; i++ {
		for j := 0; j < 2; j++ {
			img.SetRGBA(i, j, fill)
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

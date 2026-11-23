package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/iconimage"
)

func TestInstanceIconSurvivesMovingSourceAndReset(t *testing.T) {
	a, _ := testDesktop(t)
	a.store.Root = t.TempDir()
	target := a.state.Instances[1]
	original := slices.Clone(a.state.Instances)
	path := filepath.Join(t.TempDir(), "wide.png")
	src := image.NewNRGBA(image.Rect(0, 0, 12, 6))
	for y := 0; y < 6; y++ {
		for x := 0; x < 12; x++ {
			src.SetNRGBA(x, y, color.NRGBA{G: 180, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, src); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	_, data, err := iconimage.ImportPNG(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.state.SetInstanceIcon(target.ID, target.Revision, "", data); err != nil {
		t.Fatal(err)
	}
	if err := a.store.Save(a.state); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, path+".moved"); err != nil {
		t.Fatal(err)
	}
	s, err := a.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	img, err := iconimage.Decode(s.InstanceIcons[target.ID])
	if err != nil {
		t.Fatal(err)
	}
	_, g, _, center := img.At(48, 48).RGBA()
	_, _, _, border := img.At(48, 0).RGBA()
	if center != 0xffff || g != 180*257 || border != 0 || len(loadInstanceIcons(s)) != 1 {
		t.Fatal("icon content or transparency lost")
	}
	reset := s.Clone()
	if err := reset.SetInstanceIcon(target.ID, target.Revision, "", ""); err == nil {
		t.Fatal("stale icon editor accepted")
	}
	if err := reset.SetInstanceIcon(target.ID, target.Revision, data, ""); err != nil {
		t.Fatal(err)
	}
	if len(reset.InstanceIcons) != 0 || s.InstanceIcons[target.ID] != data || s.Selected != original[0].ID || !slices.Equal(s.Instances, original) {
		t.Fatal("reset or profile isolation failed")
	}
}

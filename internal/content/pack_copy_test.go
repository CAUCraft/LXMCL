package content_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

func TestCopySelectedResourcePacks(t *testing.T) {
	source := instance.Instance{ID: 1, Name: "Twin", Version: "1.21.1", Loader: "Vanilla", GameDir: t.TempDir()}
	target := instance.Instance{ID: 2, Name: source.Name, Version: source.Version, Loader: "Fabric", LoaderVersion: "0.16.14", GameDir: t.TempDir()}
	wrong := target
	wrong.Version = "1.21.4"
	if content.PackCopyTarget(source, wrong) || !content.PackCopyTarget(source, target) {
		t.Fatal("resource pack compatibility must match only the exact game version")
	}
	archive := resourceArchive(t, "pack.mcmeta")
	write := func(path string, data []byte) {
		t.Helper()
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	read := func(path string) []byte {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	src, dst := filepath.Join(source.GameDir, "resourcepacks"), filepath.Join(target.GameDir, "resourcepacks")
	if err := os.Mkdir(src, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"on.zip", "off.zip.disabled", "same.zip", "unselected.zip"} {
		write(filepath.Join(src, name), archive)
	}
	options := filepath.Join(target.GameDir, "options.txt")
	write(options, []byte("resourcePacks:[\"vanilla\"]\n"))
	files, err := content.Inventory(t.Context(), source, content.ResourcePacks)
	if err != nil {
		t.Fatal(err)
	}
	selected := slices.DeleteFunc(files, func(file content.File) bool { return file.Name == "unselected.zip" })
	plan, err := content.PreviewPackCopy(t.Context(), source, target, selected)
	if err != nil || plan.Ready != 3 || len(plan.Items) != 3 {
		t.Fatal(plan, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if n, err := plan.Apply(ctx); n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatal(n, err)
	}
	if _, err := os.Stat(dst); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("preview or cancellation created a target directory", err)
	}
	if err := os.Mkdir(dst, 0700); err != nil {
		t.Fatal(err)
	}
	conflict := filepath.Join(dst, "SAME.zip.disabled")
	write(conflict, []byte("previous file"))
	if n, err := plan.Apply(t.Context()); n != 0 || err == nil {
		t.Fatal("accepted a conflict added after preview", n, err)
	}
	plan, err = content.PreviewPackCopy(t.Context(), source, target, selected)
	if err != nil || plan.Ready != 2 {
		t.Fatal(plan, err)
	}
	if err := os.Rename(dst, dst+".original"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dst, 0700); err != nil {
		t.Fatal(err)
	}
	if n, err := plan.Apply(t.Context()); n != 0 || err == nil {
		t.Fatal("accepted a replaced target directory", n, err)
	}
	if err := os.Remove(dst); err != nil {
		t.Fatal("replacement directory was modified", err)
	}
	if err := os.Rename(dst+".original", dst); err != nil {
		t.Fatal(err)
	}
	if n, err := plan.Apply(t.Context()); n != 2 || err != nil {
		t.Fatal(n, err)
	}
	for _, name := range []string{"on.zip", "off.zip.disabled"} {
		if !bytes.Equal(read(filepath.Join(src, name)), archive) || !bytes.Equal(read(filepath.Join(dst, name)), archive) {
			t.Fatal("source or copied bytes changed", name)
		}
	}
	if got := string(read(conflict)); got != "previous file" || string(read(options)) != "resourcePacks:[\"vanilla\"]\n" {
		t.Fatal("overwrote target conflict or game selection")
	}
	entries, err := os.ReadDir(dst)
	if err != nil || len(entries) != 3 {
		t.Fatal("unselected, duplicate or partial files in destination", entries, err)
	}
}

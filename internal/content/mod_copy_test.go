package content_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

func TestCopySelectedMods(t *testing.T) {
	source := instance.Instance{ID: 1, Version: "1.21.1", Loader: "Fabric", LoaderVersion: "0.16.14", GameDir: t.TempDir()}
	target := source
	target.ID, target.GameDir = 2, t.TempDir()
	for _, change := range []func(*instance.Instance){
		func(i *instance.Instance) { i.Loader = "Quilt" },
		func(i *instance.Instance) { i.Version = "1.21.4" },
		func(i *instance.Instance) { i.ID = source.ID },
	} {
		other := target
		change(&other)
		if _, err := content.PreviewModCopy(t.Context(), source, other, nil); err == nil {
			t.Fatal("offered incompatible target", other)
		}
	}
	add := func(i instance.Instance, name, id string) {
		writeTestMod(t, i, name, "fabric.mod.json", `{"id":"`+id+`","version":"1"}`)
	}
	add(source, "a.jar.disabled", "apple")
	add(source, "b.jar", "bee")
	add(source, "c.jar", "clover")
	add(source, "d.jar", "duplicate")
	add(source, "e.jar", "duplicate")
	add(target, "c.jar.disabled", "other")
	add(target, "renamed.jar", "bee")
	files, err := content.Inventory(t.Context(), source, content.Mods)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := content.PreviewModCopy(t.Context(), source, target, files)
	if err != nil || plan.Ready != 1 || len(plan.Items) != 5 {
		t.Fatal(plan, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if n, err := plan.Apply(ctx); n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatal(n, err)
	}
	add(target, "late.jar", "apple")
	if n, err := plan.Apply(t.Context()); err == nil || n != 0 {
		t.Fatal("late ID conflict", n, err)
	}
	if err := os.Remove(filepath.Join(target.GameDir, "mods", "late.jar")); err != nil {
		t.Fatal(err)
	}
	if n, err := plan.Apply(t.Context()); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	for _, f := range plan.Items {
		if (f.File.Name == "a.jar") != (f.Conflict == "") {
			t.Fatal(f)
		}
	}
	src := filepath.Join(source.GameDir, "mods", "a.jar.disabled")
	dst := filepath.Join(target.GameDir, "mods", "a.jar.disabled")
	read := func(path string) []byte {
		t.Helper()
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	if !bytes.Equal(read(src), read(dst)) {
		t.Fatal("wrong copied bytes")
	}
	if n, err := plan.Apply(t.Context()); err == nil || n != 0 {
		t.Fatal("reused stale preview", n, err)
	}
	if err := os.Remove(dst); err != nil {
		t.Fatal(err)
	}
	add(source, "a.jar.disabled", "different")
	if n, err := plan.Apply(t.Context()); err == nil || n != 0 {
		t.Fatal("source replaced", n, err)
	}
	remaining, err := os.ReadDir(filepath.Dir(dst))
	if err != nil || len(remaining) != 2 {
		t.Fatal("partial or unselected files", remaining, err)
	}
}

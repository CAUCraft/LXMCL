package saves_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/saves"
)

func TestCopyOneWorldKeepsPreviewedName(t *testing.T) {
	source, target, world := copyWorldFixture(t)
	dst := filepath.Join(target.GameDir, "saves")
	writeCopyWorldFile(t, dst, "河谷", "level.dat", "original target")
	writeCopyWorldFile(t, dst, "河谷 (2)", "level.dat", "original second")
	p, err := saves.PreviewWorldCopy(t.Context(), source, target, world)
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(dst, "河谷 (3)")
	if p.Destination() != destination {
		t.Fatal(p.Destination())
	}
	if _, err := os.Lstat(destination); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("preview reserved a destination", err)
	}
	if err := os.Mkdir(destination, 0700); err != nil {
		t.Fatal(err)
	}
	if err := p.Apply(t.Context()); !errors.Is(err, fs.ErrExist) {
		t.Fatal("apply continued after the previewed name was claimed", err)
	}
	if members, err := os.ReadDir(destination); err != nil || len(members) != 0 {
		t.Fatal("changed a pre-existing empty directory", members, err)
	}
	if err := os.Remove(destination); err != nil {
		t.Fatal(err)
	}
	if err := p.Apply(t.Context()); err != nil {
		t.Fatal(err)
	}
	for file, want := range map[string]string{
		filepath.Join(destination, "level.dat"):                             "source level",
		filepath.Join(destination, "region", "r.0.0.mca"):                   "source region",
		filepath.Join(dst, "河谷", "level.dat"):                               "original target",
		filepath.Join(dst, "河谷 (2)", "level.dat"):                           "original second",
		filepath.Join(source.GameDir, "saves", "河谷", "level.dat"):           "source level",
		filepath.Join(source.GameDir, "saves", "河谷", "session.lock"):        "session lock",
		filepath.Join(source.GameDir, "saves", "河谷", "region", "r.0.0.mca"): "source region",
	} {
		got, err := os.ReadFile(file)
		if err != nil || string(got) != want {
			t.Error(file, string(got), err)
		}
	}
	if _, err := os.Stat(filepath.Join(destination, "session.lock")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("copied the source session lock", err)
	}
	worlds, err := saves.Worlds(t.Context(), target.GameDir)
	if err != nil || len(worlds) != 3 || worlds[2].Folder() != "河谷 (3)" {
		t.Fatal(worlds, err)
	}
}

func TestCopyWorldDetectsSourceReplacement(t *testing.T) {
	source, target, world := copyWorldFixture(t)
	p, err := saves.PreviewWorldCopy(t.Context(), source, target, world)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(source.GameDir, "saves", world.Folder(), "region", "r.0.0.mca")
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	// Keep the original live so the file system cannot reuse its ID.
	if err := os.Rename(file, filepath.Join(source.GameDir, "held-region")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("second region"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(file, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := p.Apply(t.Context()); err == nil {
		t.Fatal("accepted a same-size source replacement with identical mtime")
	}
	if _, err := os.Stat(filepath.Join(target.GameDir, "saves")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("created saves despite stale source", err)
	}
}

func TestCopyWorldCancellationRemovesOnlyOwnedOutput(t *testing.T) {
	source, target, world := copyWorldFixture(t)
	p, err := saves.PreviewWorldCopy(t.Context(), source, target, world)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(target.GameDir, "saves")
	if _, err := os.Stat(dst); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("preview created saves", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	afterStage := &cancelWorldCopy{Context: ctx, cancel: cancel, stage: filepath.Join(p.Destination(), "region", "r.0.0.mca")}
	if err := p.Apply(afterStage); !afterStage.staged || !errors.Is(err, context.Canceled) {
		t.Fatal("expected cancellation after the reserved directory was populated", afterStage.staged, err)
	}
	if _, err := os.Stat(dst); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("cancel left a partial world or a newly created saves directory", err)
	}
	if err := p.Apply(t.Context()); err != nil {
		t.Fatal("copy into a registered instance without saves", err)
	}
}

type cancelWorldCopy struct {
	context.Context
	cancel context.CancelFunc
	stage  string
	staged bool
}

func (c *cancelWorldCopy) Err() error {
	if c.Context.Err() == nil {
		if _, err := os.Stat(c.stage); err == nil {
			c.staged = true
			c.cancel()
		}
	}
	return c.Context.Err()
}

func TestWorldCopyCompatibility(t *testing.T) {
	source, target, _ := copyWorldFixture(t)
	check := func(want bool) {
		t.Helper()
		if got := saves.WorldCopyCompatible(source, target); got != want {
			t.Fatalf("compatible = %v for %+v and %+v", got, source, target)
		}
	}
	check(true)
	target.Loader = instance.LoaderVanilla
	check(true)
	target.Version = "1.21.4"
	check(false)
	target.Version = source.Version
	target.Loader = instance.LoaderForge
	check(false)
	source.Loader, source.LoaderVersion = instance.LoaderForge, "52.1.0"
	target.LoaderVersion = "52.1.1"
	check(false)
	target.Loader, target.LoaderVersion = "forge", source.LoaderVersion
	check(true)
	target.ID = source.ID
	check(false)
	target.ID, target.GameDir = 2, source.GameDir
	check(false)
}

func copyWorldFixture(t *testing.T) (instance.Instance, instance.Instance, saves.World) {
	t.Helper()
	base := t.TempDir()
	source := instance.Instance{ID: 1, Name: "source", Version: "1.21.1", GameDir: filepath.Join(base, "source"), Revision: 2}
	target := instance.Instance{ID: 2, Name: "destination", Version: source.Version, GameDir: filepath.Join(base, "target"), Revision: 4}
	if err := os.Mkdir(target.GameDir, 0700); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(source.GameDir, "saves")
	writeCopyWorldFile(t, dir, "河谷", "level.dat", "source level")
	writeCopyWorldFile(t, dir, "河谷", filepath.Join("region", "r.0.0.mca"), "source region")
	writeCopyWorldFile(t, dir, "河谷", "session.lock", "session lock")
	worlds, err := saves.Worlds(t.Context(), source.GameDir)
	if err != nil || len(worlds) != 1 {
		t.Fatal(worlds, err)
	}
	return source, target, worlds[0]
}

func writeCopyWorldFile(t *testing.T, root, folder, name, value string) {
	t.Helper()
	file := filepath.Join(root, folder, name)
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
}

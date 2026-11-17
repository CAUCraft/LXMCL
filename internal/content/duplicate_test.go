package content

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/tasks"
)

func TestDuplicateSetupPublishesOnlyChosenContent(t *testing.T) {
	source := t.TempDir()
	for _, name := range []string{"options.txt", "OptionsOF.txt", "mods/test.jar.disabled", "config/a/b.cfg", "saves/World/level.dat", "logs/latest.log"} {
		path := filepath.Join(source, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	parent := t.TempDir()
	target := filepath.Join(parent, "instance-2")
	failedSave := errors.New("save failed")
	err := DuplicateSetup(t.Context(), source, target, func() error { return failedSave }, nil)
	if !errors.Is(err, failedSave) {
		t.Fatalf("lost commit error: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	commit := func() error { t.Fatal("unexpected commit"); return nil }
	err = DuplicateSetup(ctx, source, target, commit, func(tasks.Progress) { cancel() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("lost cancellation: %v", err)
	}
	if entries, err := os.ReadDir(parent); err != nil || len(entries) != 0 {
		t.Fatalf("failed copies left files: %v %v", entries, err)
	}
	if err := DuplicateSetup(t.Context(), source, target, func() error { return nil }, nil); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"options.txt", "mods/test.jar.disabled", "config/a/b.cfg"} {
		b, err := os.ReadFile(filepath.Join(target, name))
		if err != nil || string(b) != name {
			t.Fatalf("copied %s: %q %v", name, b, err)
		}
	}
	_, lookupErr := os.Stat(filepath.Join(source, "optionsof.txt"))
	copied, copyErr := os.ReadFile(filepath.Join(target, "optionsof.txt"))
	if lookupErr == nil && (copyErr != nil || string(copied) != "OptionsOF.txt") ||
		errors.Is(lookupErr, os.ErrNotExist) && !errors.Is(copyErr, os.ErrNotExist) {
		t.Fatalf("copy disagrees with filesystem name lookup: %v %v", lookupErr, copyErr)
	}
	for _, name := range []string{"saves", "logs"} {
		if _, err := os.Stat(filepath.Join(target, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s copied: %v", name, err)
		}
	}
	if err := DuplicateSetup(t.Context(), source, target, commit, nil); err == nil {
		t.Fatal("accepted existing destination")
	}
	if err := DuplicateSetup(t.Context(), source, filepath.Join(source, "nested", "copy"), commit, nil); err == nil {
		t.Fatal("accepted nested destination")
	}
	if _, err := os.Stat(filepath.Join(source, "nested")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("changed source: %v", err)
	}
	if err := os.Symlink("saves", filepath.Join(source, "scripts")); err == nil {
		if err := DuplicateSetup(t.Context(), source, filepath.Join(parent, "linked"), commit, nil); err == nil {
			t.Fatal("followed linked resources")
		}
	}
}

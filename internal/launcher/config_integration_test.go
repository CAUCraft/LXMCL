package launcher

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/tasks"
)

func TestConfigMigrationDoesNotRebindTaskRetry(t *testing.T) {
	store := Store{Root: t.TempDir()}
	i := Instance{ID: 1, Name: "submitted", Version: "1.21.1", GameDir: filepath.Join(store.Root, "games", "instance-1"), Revision: 7}
	data, err := json.Marshal(map[string]any{"instances": []Instance{i}, "selected": 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Root, "launcher.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	next, created, err := state.PrepareInstallation("new", "1.21.1", LoaderVanilla, "")
	if err != nil || created.ID != 2 || created.GameDir == i.GameDir || next.Selected != 1 {
		t.Fatalf("migration reused target: %+v %v", created, err)
	}
	r := tasks.Record{InstanceID: i.ID, Revision: i.Revision, Version: i.Version, Action: tasks.Launch, Status: tasks.Failed, Account: state.AccountKey()}
	if got, ok := RetryTarget(r, next); !ok || got != i {
		t.Fatalf("old retry changed target: %+v %v", got, ok)
	}
	if err := next.Rename(i.ID, i.Revision, "updated"); err != nil {
		t.Fatal(err)
	}
	if _, ok := RetryTarget(r, next); ok {
		t.Fatal("stale retry bypassed revision check")
	}
}

func TestNewInstanceDirectoryRegistration(t *testing.T) {
	store := Store{Root: t.TempDir()}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	next, i, err := state.PrepareInstallation("new", "1.21.1", LoaderVanilla, "")
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(i.GameDir, "saves", "world", "level.dat")
	if err := os.MkdirAll(filepath.Dir(marker), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte("old world"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.RegisterInstance(next, i); err == nil {
		t.Fatal("registered an existing game directory")
	}
	if b, err := os.ReadFile(marker); err != nil || string(b) != "old world" {
		t.Fatalf("existing world changed: %q, %v", b, err)
	}
	state.Settings.GameRoot = filepath.Join(store.Root, "new-root")
	next, i, err = state.PrepareInstallation("new", "1.21.1", LoaderVanilla, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := (Store{Root: marker}).RegisterInstance(next, i); err == nil {
		t.Fatal("registration succeeded with an unusable config directory")
	}
	if _, err := os.Stat(i.GameDir); !os.IsNotExist(err) {
		t.Fatalf("failed config write left a directory reservation: %v", err)
	}
	t.Run("linked parent of an existing profile", func(t *testing.T) {
		link := filepath.Join(store.Root, "legacy-root")
		if err := os.Symlink(next.Settings.GameRoot, link); err != nil {
			t.Skipf("directory links unavailable: %v", err)
		}
		claimed := next.Clone()
		claimed.Instances = append(claimed.Instances, Instance{ID: 7, GameDir: filepath.Join(link, filepath.Base(i.GameDir))})
		if err := store.RegisterInstance(claimed, i); err == nil {
			t.Fatal("registered a second profile for the same directory through a link")
		}
	})
	if err := store.RegisterInstance(next, i); err != nil {
		t.Fatal(err)
	}
}

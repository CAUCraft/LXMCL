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

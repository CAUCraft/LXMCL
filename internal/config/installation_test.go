package config_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/account"
	"github.com/CAUCraft/LXMCL/internal/config"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/server"
)

func TestInstallationDraftIsIndependentAndKeepsLaunchSelection(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-created")
	state := config.State{Settings: instance.Settings{GameRoot: root}, NextID: 2, Selected: 1,
		Instances:      []instance.Instance{{ID: 1, Name: "existing", Version: "1.21.1", GameDir: filepath.Join(root, "instance-1")}},
		OfflinePlayers: []string{"Player"}, StationProfiles: []account.StationProfile{{Key: "station"}}, Servers: []server.Server{{Name: "server", Address: "example.com"}}}
	before := state.Clone()
	next, target, err := state.PrepareInstallation("new", "1.21.1", instance.LoaderForge, "1.21.1-52.1.14")
	if err != nil || next.Selected != 1 || target.ID != 2 || target.LoaderVersion != "52.1.14" || target.Revision != 2 || !reflect.DeepEqual(before, state) {
		t.Fatalf("invalid installation draft: %+v %+v %v", next, target, err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("planning touched game directory: %v", err)
	}
	next.Instances[0].Name = "changed"
	next.OfflinePlayers[0] = "changed"
	next.StationProfiles[0].Key = "changed"
	next.Servers[0].Name = "changed"
	if !reflect.DeepEqual(state, before) {
		t.Fatal("draft shares writable state with original")
	}
	failed, target, err := state.PrepareInstallation("invalid", "1.21.1", instance.LoaderFabric, "bad version")
	if err == nil || target != (instance.Instance{}) || !reflect.DeepEqual(failed, before) || !reflect.DeepEqual(state, before) {
		t.Fatalf("failed planning allocated or modified state: %+v %v", failed, err)
	}
	first := config.State{Settings: instance.Settings{GameRoot: root}}
	prepared, target, err := first.PrepareInstallation("first", "1.21.1", instance.LoaderVanilla, "")
	if err != nil || prepared.Selected != 1 || target.ID != 1 || target.Revision != 1 || len(first.Instances) != 0 {
		t.Fatalf("first selection: %+v %v", prepared, err)
	}
}

package launcher

import (
	"testing"

	"github.com/CAUCraft/LXMCL/internal/tasks"
)

func TestResourceRetryKeepsOriginalInstanceRevision(t *testing.T) {
	i := Instance{ID: 3, Revision: 1, Version: "1.21.1", Loader: "Fabric", LoaderVersion: "0.16.14", GameDir: t.TempDir()}
	state := State{Instances: []Instance{i}, Selected: 4}
	row := tasks.Record{InstanceID: i.ID, Revision: i.Revision, Version: i.Version, Status: tasks.Canceled,
		Resource: &tasks.Resource{Kind: "mods", Project: "modA", Version: "releaseA"}}
	for _, kind := range []string{"mods", "resourcepacks", "shaderpacks"} {
		row.Resource.Kind = kind
		if target, ok := RetryTarget(row, state); !ok || target.ID != i.ID {
			t.Fatalf("%s retry did not use its original instance", kind)
		}
	}
	for _, kind := range []string{"", "unknown"} {
		row.Resource.Kind = kind
		if _, ok := RetryTarget(row, state); ok {
			t.Fatalf("unsupported resource kind %q was accepted", kind)
		}
	}
	row.Resource.Kind = "mods"
	state.Instances[0].Revision++
	if _, ok := RetryTarget(row, state); ok {
		t.Fatal("resource retry was accepted after an instance revision change")
	}
}

package main

import (
	"reflect"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/launcher"
)

func TestInstallRejectsRenamedGroupAtSubmission(t *testing.T) {
	a, _ := testDesktop(t)
	a.ctx = t.Context()
	a.store.Root = t.TempDir()
	t.Cleanup(a.writes.Wait)
	a.releases = []launcher.Version{{ID: "1.21.4"}}
	a.state.InstanceGroups = map[int]string{1: "生存"}
	a.install.version, a.install.group = "1.21.4", "生存"
	if err := a.state.RenameInstanceGroup("生存", "生存测试"); err != nil {
		t.Fatal(err)
	}
	before := a.state.Clone()
	for _, installNow := range []bool{false, true} {
		a.submitInstall(&a.install, installNow)
		if a.busy || !reflect.DeepEqual(a.state, before) || a.install.group != "生存" {
			t.Fatalf("submission with an obsolete group changed state or started work (installNow = %v)", installNow)
		}
	}
}

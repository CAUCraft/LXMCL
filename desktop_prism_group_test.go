package main

import (
	"context"
	"reflect"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/content"
)

func TestPrismImportRejectsRenamedGroupAtSubmission(t *testing.T) {
	a, _ := testDesktop(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	a.ctx = ctx
	t.Cleanup(a.writes.Wait)

	a.state.InstanceGroups = map[int]string{1: "生存"}
	a.startPrismImport()
	d := a.form.prism
	d.source = content.PrismInstance{Version: "1.21.1"}
	a.form.name, d.group = "待导入实例", "生存"
	if err := a.state.RenameInstanceGroup("生存", "生存测试"); err != nil {
		t.Fatal(err)
	}

	before := a.state.Clone()
	a.importPrism(d)
	if a.busy || !a.form.open || !reflect.DeepEqual(a.state, before) || d.group != "生存" {
		t.Fatal("import accepted an obsolete group or discarded the confirmation")
	}
}

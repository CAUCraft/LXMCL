package main

import (
	"context"
	"reflect"
	"testing"
)

func TestModpackCreationRejectsMissingGroupAndExistingTarget(t *testing.T) {
	a, _ := testDesktop(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	a.ctx = ctx
	t.Cleanup(a.writes.Wait)

	a.state.InstanceGroups = map[int]string{1: "生存"}
	p := &a.modpacks
	p.prepared = prepareDesktopTestPack(t, "group.mrpack", "modrinth.index.json",
		`{"formatVersion":1,"game":"minecraft","name":"Fixture","versionId":"1","dependencies":{"minecraft":"1.21.1"}}`)
	p.name, p.group = "新实例", "生存"
	if err := a.state.RenameInstanceGroup("生存", "生存测试"); err != nil {
		t.Fatal(err)
	}
	before := a.state.Clone()
	a.installModpack()
	if a.busy || !reflect.DeepEqual(a.state, before) || p.group != "生存" {
		t.Fatal("creation accepted an obsolete group")
	}
	p.group, p.target = "生存测试", "1"
	a.installModpack()
	if a.busy || !reflect.DeepEqual(a.state, before) {
		t.Fatal("new-instance installation accepted an existing target")
	}
}

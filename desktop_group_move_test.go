package main

import "testing"

func TestGroupMoveRejectsChangedMemberWithoutSavingOtherRows(t *testing.T) {
	a, _ := testDesktop(t)
	a.state.InstanceGroups = map[int]string{1: "First", 2: "Second"}
	a.openGroupMove()
	p := a.form.groupMove
	for n := range p.rows {
		p.rows[n].checked = true
	}
	a.state.InstanceGroups[2] = "Updated"
	a.submitEditor()
	if a.form.err == "" || a.busy || a.state.InstanceGroups[1] != "First" || a.state.InstanceGroups[2] != "Updated" {
		t.Fatalf("stale selection changed instance groups: %v, %q", a.state.InstanceGroups, a.form.err)
	}
}

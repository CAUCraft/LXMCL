package main

import (
	"slices"
	"testing"
)

func TestTaskHistoryFiltersUseRecordedInstanceIDs(t *testing.T) {
	a := &desktop{history: []taskRecord{
		{ID: "two", InstanceID: 2, Name: "同名实例", Status: taskCompleted},
		{ID: "legacy", Name: "同名实例", Status: taskFailed},
		{ID: "one-fail", InstanceID: 1, Name: "同名实例", Status: taskFailed},
		{ID: "one-old", InstanceID: 1, Name: "更名前", Status: taskCompleted},
	}}
	check := func(want ...string) {
		t.Helper()
		var got []string
		for _, row := range a.visibleTaskHistory() {
			got = append(got, row.Key())
		}
		if !slices.Equal(got, want) {
			t.Fatalf("visible history = %v, want %v", got, want)
		}
	}
	check("two", "legacy", "one-fail", "one-old")
	a.taskInstanceFilter = 1
	check("one-fail", "one-old")
	a.taskFilter = 1
	check("one-fail")
	a.taskInstanceFilter = 2
	check()

	a.noticeTaskKey = "legacy"
	a.openNoticeTask()
	check("legacy", "one-fail")
	if a.browsedTask != "legacy" || a.taskInstanceFilter != 0 || a.taskFilter != 1 {
		t.Fatal("revealing the no-ID task did not preserve the applicable status filter")
	}
}

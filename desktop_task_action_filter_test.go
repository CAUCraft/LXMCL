package main

import (
	"slices"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/tasks"
)

func TestTaskHistoryActionUsesRecordedDiscriminators(t *testing.T) {
	a := &desktop{history: []taskRecord{
		{ID: "other-instance", Name: "同名", Action: tasks.Launch, InstanceID: 2, Status: taskFailed},
		{ID: "started", Name: "同名", Action: tasks.Launch, InstanceID: 1, Status: taskExited},
		{ID: "failed", Name: "同名", Action: tasks.Launch, InstanceID: 1, Status: taskFailed},
		{ID: "backup", Name: "同名", Action: tasks.Backup, InstanceID: 1, Status: taskCanceled},
		{ID: "mod", Name: "同名", Action: tasks.ModUpdateAction, Status: taskCompleted},
		{ID: "prefix", Name: "同名", Action: tasks.Launch + " · 旧标题", InstanceID: 1, Status: taskFailed},
		{ID: "legacy", Name: tasks.Launch, InstanceID: 1, Status: taskFailed},
	}}
	check := func(want ...string) {
		t.Helper()
		var keys []string
		for _, row := range a.visibleTaskHistory() {
			keys = append(keys, row.Key())
		}
		if !slices.Equal(keys, want) {
			t.Fatalf("visible tasks = %v, want %v", keys, want)
		}
	}
	check("other-instance", "started", "failed", "backup", "mod", "prefix", "legacy")
	a.taskActionFilter = tasks.Launch
	check("other-instance", "started", "failed")
	a.taskInstanceFilter, a.taskFilter = 1, 1
	a.taskHistoryQuery, a.taskActionFilter = "同名", tasks.Launch
	check("failed")
	a.noticeTaskKey = "backup"
	a.openNoticeTask()
	check("failed", "backup", "prefix")
	if a.taskActionFilter != "" || a.taskHistoryQuery != "同名" || a.taskFilter != 1 || a.taskInstanceFilter != 1 ||
		a.browsedTask != "backup" {
		t.Fatal("opening a notice changed filters that allowed its exact record")
	}
	a.taskActionFilter = tasks.Backup
	a.openNoticeTask()
	check("backup")
	if a.taskActionFilter != tasks.Backup {
		t.Fatal("opening a matching notice cleared the action filter")
	}
}

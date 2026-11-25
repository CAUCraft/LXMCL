package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CAUCraft/LXMCL/internal/tasks"
	"github.com/egoist/mygo/ui"
)

func TestInstanceOverviewShowsLastFinishedLaunch(t *testing.T) {
	a, tt := testDesktop(t)
	a.state.Instances[0].Name, a.state.Instances[1].Name = "同名实例", "同名实例"
	first, second := a.state.Instances[0], a.state.Instances[1]
	empty := second
	empty.ID, empty.Name, empty.GameDir = 3, "仅有未结束任务", t.TempDir()
	a.state.NextID = 4
	a.state.Instances = append(a.state.Instances, empty)
	at := time.Date(2026, time.November, 25, 8, 0, 0, 0, time.UTC)
	failed := taskRecord{
		ID: "latest-failure", InstanceID: second.ID, Name: second.Name,
		Action: taskLaunch, Status: taskFailed, Detail: "启动前失败的测试详情", At: at,
	}
	exited := taskRecord{ID: "exit-a", InstanceID: first.ID, Name: first.Name, Action: taskLaunch, Status: taskExited, At: at.Add(4 * time.Minute)}
	previous := taskRecord{ID: "verify-a", InstanceID: first.ID, Name: first.Name, Action: taskVerify, Status: taskFailed, At: at.Add(time.Minute)}
	records := []taskRecord{
		{ID: "running-b", InstanceID: second.ID, Name: second.Name, Action: taskLaunch, Status: taskStarted, At: at.Add(8 * time.Minute), PID: 1234},
		{ID: "verify-b", InstanceID: second.ID, Name: second.Name, Action: taskVerify, Status: taskCompleted, At: at.Add(7 * time.Minute)},
		{ID: "legacy", Name: second.Name, Action: taskLaunch, Status: taskExited, At: at.Add(6 * time.Minute)},
		{ID: "pending-c", InstanceID: empty.ID, Name: empty.Name, Action: taskLaunch, Status: taskStarted, At: at.Add(5 * time.Minute)},
		exited, previous,
		{ID: "older-failure", InstanceID: second.ID, Name: second.Name, Action: taskLaunch, Status: taskFailed, At: at.Add(-time.Minute), Detail: "更早的测试失败"},
		failed,
	}
	a.store.Root = t.TempDir()
	if err := tasks.Save(a.store.Root, records); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(a.store.Root, "tasks.json")
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := tasks.Load(a.store.Root)
	if err != nil {
		t.Fatal(err)
	}
	a.restoreTaskHistory(loaded)
	a.taskFilter, a.taskInstanceFilter = 1, first.ID
	a.taskHistoryQuery, a.browsedTask = "校验", previous.Key()
	tt.SetSize(820, 530)

	click := func(labels ...string) {
		t.Helper()
		for _, label := range labels {
			if err := tt.Click(label); err != nil {
				t.Fatalf("%s: %v (%v)", label, err, tt.Texts())
			}
			tt.Frame()
		}
	}
	result := func(row taskRecord) {
		t.Helper()
		want := row.Status + " · " + row.At.Local().Format("2006-01-02 15:04")
		if !tt.HasText("上次启动结果") || !tt.HasText(want) || !tt.HasText("查看启动任务") {
			t.Fatal("missing expected result", want, tt.Texts())
		}
	}
	returned := func() {
		t.Helper()
		if a.page != pageLibrary || a.detailPage != detailOverview || a.inspected != second.ID ||
			a.state.Selected != first.ID || a.instanceTaskHistory != nil || a.taskFilter != 1 ||
			a.taskInstanceFilter != first.ID || a.taskHistoryQuery != "校验" || a.browsedTask != previous.Key() {
			t.Fatal("return lost the instance overview or ordinary task filters")
		}
	}

	click("实例", "1.21.4 · 原版")
	result(failed)
	if tt.HasText(failed.Detail) {
		t.Fatal("the overview included task details")
	}
	capture(t, tt, "instance-last-launch-failure-light")
	click("查看启动任务")
	if a.taskInstanceFilter != second.ID || a.taskFilter != 0 || a.taskHistoryQuery != "" ||
		a.browsedTask != failed.Key() || !tt.HasText(failed.Detail) || tt.HasText("更早的测试失败") {
		t.Fatal("did not open the exact launch task", tt.Texts())
	}
	click("返回实例概览")
	returned()

	canceled := failed
	canceled.ID, canceled.Status, canceled.Detail, canceled.At = "latest-cancel", taskCanceled, "", at.Add(2*time.Minute)
	a.history = append([]taskRecord{canceled}, a.history...)
	tt.SetDark(true)
	tt.Frame()
	tt.Advance(400 * time.Millisecond)
	result(canceled)
	capture(t, tt, "instance-last-launch-canceled-dark")
	click("查看启动任务")
	if a.browsedTask != canceled.Key() || a.taskInstanceFilter != second.ID {
		t.Fatal("did not locate the canceled attempt")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	returned()

	ambiguous := canceled
	ambiguous.ID, ambiguous.At = "", at.Add(3*time.Minute)
	sameKey := ambiguous
	sameKey.Action = taskVerify
	a.history = append([]taskRecord{ambiguous, sameKey}, a.history...)
	tt.Frame()
	result(ambiguous)
	if entry, ok := tt.Access("查看启动任务"); !ok || !entry.Disabled {
		t.Fatal("a duplicate legacy key was offered as an exact target")
	}

	click("1.21.1 · 原版 · 启动目标")
	result(exited)
	click(empty.Name)
	if !tt.HasText("暂无已结束的启动记录") || tt.HasText("查看启动任务") {
		t.Fatal("an unfinished launch appeared as a result", tt.Texts())
	}
	a.restoreTaskHistory(nil)
	tt.Frame()
	if !tt.HasText("暂无已结束的启动记录") || tt.HasText("查看启动任务") {
		t.Fatal("the empty history did not show the empty state", tt.Texts())
	}
	after, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(before, after) || a.state.Selected != first.ID || a.busy || a.task.Stage != "" {
		t.Fatal("viewing results wrote history, changed target, or started a task", err)
	}
}

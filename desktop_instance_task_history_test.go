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

func TestInstanceOverviewOpensItsTaskHistory(t *testing.T) {
	a, tt := testDesktop(t)
	a.state.Instances[0].Name, a.state.Instances[1].Name = "同名实例", "同名实例"
	first, second := a.state.Instances[0], a.state.Instances[1]
	empty := second
	empty.ID, empty.Name, empty.GameDir = 3, "无历史实例", t.TempDir()
	a.state.NextID = 4
	a.state.Instances = append(a.state.Instances, empty)
	a.store.Root = t.TempDir()
	at := time.Date(2026, time.November, 3, 8, 0, 0, 0, time.UTC)
	rows := []taskRecord{
		{ID: "b-completed", InstanceID: second.ID, Name: second.Name, Action: "校验 B 已完成", Status: taskCompleted, Detail: "B 的已完成记录", At: at},
		{ID: "a-failed", InstanceID: first.ID, Name: first.Name, Action: "校验 A 失败", Status: taskFailed, Detail: "A 的原失败详情", At: at.Add(-time.Minute)},
	}
	if err := tasks.Save(a.store.Root, rows); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(a.store.Root, "tasks.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := tasks.Load(a.store.Root)
	if err != nil {
		t.Fatal(err)
	}
	a.restoreTaskHistory(loaded)
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
	themes := func(stem string) {
		t.Helper()
		for _, theme := range []string{"light", "dark"} {
			tt.SetDark(theme == "dark")
			tt.Advance(300 * time.Millisecond)
			tt.Frame()
			capture(t, tt, stem+"-"+theme)
		}
	}

	click("任务中心", "失败 / 取消", "筛选历史实例", "#1 · 同名实例", "实例", "1.21.4 · 原版")
	if a.inspected != second.ID || a.state.Selected != first.ID {
		t.Fatal("could not browse the second same-name instance without changing the home target")
	}
	themes("instance-task-history-entry")
	click("查看任务历史")
	if a.taskFilter != 0 || a.taskInstanceFilter != second.ID || a.state.Selected != first.ID ||
		!tt.HasText("校验 B 已完成") || tt.HasText("A 的原失败详情") || len(a.visibleTaskHistory()) != 1 {
		t.Fatal("overview shortcut did not show only the second instance's completed record", tt.Texts())
	}
	themes("instance-task-history-scoped")
	click("返回实例概览")
	if a.page != pageLibrary || a.inspected != second.ID || a.detailPage != detailOverview {
		t.Fatal("return button did not restore the second instance's overview")
	}
	click("任务中心")
	if a.taskFilter != 1 || a.taskInstanceFilter != first.ID || a.instanceTaskHistory != nil ||
		!tt.HasText("A 的原失败详情") {
		t.Fatal("ordinary task entry did not restore the previous history filters", tt.Texts())
	}
	click("实例", "查看任务历史")
	a.notice, a.noticeError, a.noticeTaskKey = "A 校验失败", true, rows[1].Key()
	tt.Frame()
	click("查看任务")
	if a.instanceTaskHistory != nil || a.browsedTask != rows[1].Key() || !tt.HasText("A 的原失败详情") || a.state.Selected != first.ID {
		t.Fatal("notification did not reveal its exact original task", tt.Texts())
	}
	click("关闭通知", "实例", "无历史实例", "查看任务历史")
	assertEmpty := func() {
		t.Helper()
		if !tt.HasText("此实例暂无任务记录") || !tt.HasText("#3 · 无历史实例") {
			t.Fatal("missing instance-specific empty state", tt.Texts())
		}
	}
	assertEmpty()
	themes("instance-task-history-empty")
	click("返回实例概览")
	a.restoreTaskHistory(nil)
	tt.Frame()
	click("查看任务历史")
	assertEmpty()
	tt.Key(0, ui.KeyEscape)
	if a.page != pageLibrary || a.inspected != empty.ID || a.detailPage != detailOverview || a.state.Selected != first.ID {
		t.Fatal("return did not restore the empty instance's overview")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) || a.busy || a.task.Stage != "" {
		t.Fatal("viewing history wrote tasks or started work", err)
	}
}

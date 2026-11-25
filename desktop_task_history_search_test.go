package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/CAUCraft/LXMCL/internal/tasks"
	"github.com/egoist/mygo/ui"
)

func TestTaskHistorySearchFromTaskCenter(t *testing.T) {
	a, tt := testDesktop(t)
	a.state.Instances[0].Name, a.state.Instances[1].Name = "同名实例", "同名实例"
	a.store.Root = t.TempDir()
	at := time.Date(2026, time.November, 25, 10, 0, 0, 0, time.UTC)
	rows := []taskRecord{
		{ID: "a-check", InstanceID: 1, Name: "同名实例 · Cache", Action: "校验文件", Status: taskFailed, Detail: "A 的校验记录", At: at},
		{ID: "b-check", InstanceID: 2, Name: "同名实例 · Cache", Action: "校验文件", Status: taskFailed, Detail: "B 的校验记录", At: at.Add(-time.Minute)},
		{ID: "b-done", InstanceID: 2, Name: "同名实例 · Build", Action: "校验文件", Status: taskCompleted, Detail: "B 的完成记录", At: at.Add(-2 * time.Minute)},
		{ID: "b-backup", InstanceID: 2, Name: "同名实例", Action: taskBackup, Status: taskFailed, Detail: "不应索引的错误原文", Log: "private-marker.log", Folder: "private-marker", At: at.Add(-3 * time.Minute)},
	}
	if err := tasks.Save(a.store.Root, rows); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(a.store.Root, "tasks.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	a.restoreTaskHistory(rows)
	tt.SetSize(820, 530)
	tt.SetPreferences(ui.Preferences{})

	click := func(labels ...string) {
		t.Helper()
		for _, label := range labels {
			if err := tt.Click(label); err != nil {
				t.Fatalf("%s: %v (%v)", label, err, tt.Texts())
			}
			tt.Frame()
		}
	}
	check := func(want ...string) {
		t.Helper()
		var got []string
		for _, row := range a.visibleTaskHistory() {
			got = append(got, row.Key())
		}
		if !slices.Equal(got, want) {
			t.Fatalf("visible history = %v, want %v (%v)", got, want, tt.Texts())
		}
	}
	const searchLabel = "搜索历史任务"
	search := func(query string) {
		t.Helper()
		tt.Key(ui.Cmd, ui.KeyF)
		if !tt.Focused(searchLabel) {
			t.Fatal("Ctrl+F did not focus the task search")
		}
		tt.Key(ui.Cmd, ui.KeyA)
		if query == "" {
			tt.Key(0, ui.KeyBackspace)
		} else {
			tt.Type(query)
		}
		tt.Frame()
	}

	click("任务中心", "失败 / 取消", "筛选历史实例", "#2 · 同名实例")
	backupLabel := "同名实例 · 备份存档 · 已失败 · " + rows[3].At.Local().Format("01-02 15:04")
	click("选择历史任务", backupLabel)
	check("b-check", "b-backup")
	search("  cAcHe ")
	check("b-check")
	if a.browsedTask != rows[3].Key() || !tt.HasText("B 的校验记录") {
		t.Fatal("search replaced the saved selection or showed another instance's record", tt.Texts())
	}
	tt.Advance(500 * time.Millisecond)
	capture(t, tt, "task-history-search-match-light")

	search("存档")
	check("b-backup")
	for _, query := range []string{"错误原文", "private-marker", "没有这样的任务"} {
		search(query)
		check()
		if !tt.HasText("没有符合搜索条件的任务") {
			t.Fatal("missing search empty state", tt.Texts())
		}
	}
	tt.SetDark(true)
	tt.Advance(500 * time.Millisecond)
	capture(t, tt, "task-history-search-empty-dark")
	search("")
	check("b-check", "b-backup")
	if a.browsedTask != rows[3].Key() || !tt.HasText(rows[3].Detail) {
		t.Fatal("keyboard clearing did not restore the selected backup", tt.Texts())
	}

	search("没有这样的任务")
	click("清除搜索")
	check("b-check", "b-backup")

	search("cache")
	click("实例", "1.21.4 · 原版", "查看任务历史")
	check("b-check", "b-done", "b-backup")
	search("build")
	check("b-done")
	tt.Key(0, ui.KeyEscape)
	if a.page != pageLibrary || a.inspected != 2 || a.detailPage != detailOverview {
		t.Fatal("Escape did not return to the same instance overview")
	}
	click("任务中心")
	check("b-check")
	if a.taskHistoryQuery != "cache" || a.taskFilter != 1 || a.taskInstanceFilter != 2 {
		t.Fatal("returning from the overview did not restore ordinary search and filters")
	}

	a.notice, a.noticeError, a.noticeTaskKey = "备份需要重试", true, rows[3].Key()
	tt.Frame()
	click("查看任务")
	check("b-check", "b-backup")
	if a.browsedTask != rows[3].Key() || !tt.HasText(rows[3].Detail) || a.taskHistoryQuery != "" ||
		a.taskFilter != 1 || a.taskInstanceFilter != 2 || a.state.Selected != 1 {
		t.Fatal("notice did not reveal the exact record and preserve applicable filters", tt.Texts())
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) || a.busy || a.task.Stage != "" {
		t.Fatal("search wrote history or started work", err)
	}
}

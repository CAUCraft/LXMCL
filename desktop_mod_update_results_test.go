package main

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/CAUCraft/LXMCL/internal/tasks"
)

func TestModUpdateResultsExecution(t *testing.T) {
	for _, tc := range []struct {
		name      string
		attempts  []error
		cancelAt  int
		want      []string
		taskState string
	}{
		{"all updated", []error{nil, nil}, 0, []string{taskCompleted, taskCompleted}, taskCompleted},
		{"continued after failure", []error{errors.New("private-path"), nil}, 0, []string{taskFailed, taskCompleted}, taskFailed},
		{"canceled current", []error{nil, errors.New("private-path"), context.Canceled}, 3, []string{taskCompleted, taskFailed, taskCanceled, tasks.ModUpdateNotRun}, taskCanceled},
		{"canceled between files", []error{nil}, 1, []string{taskCompleted, tasks.ModUpdateNotRun}, taskCanceled},
		{"last file committed", []error{nil}, 1, []string{taskCompleted}, taskCompleted},
		{"canceled before work", nil, -1, []string{tasks.ModUpdateNotRun}, taskCanceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tc.cancelAt == -1 {
				cancel()
			}
			items := make([]tasks.ModUpdateItem, len(tc.want))
			for n := range items {
				items[n].Status = tasks.ModUpdateNotRun
			}
			attempts := 0
			err := tasks.Run(ctx, func(ctx context.Context, _ func(tasks.Progress)) error {
				return runModUpdateBatch(ctx, items, func(n int) error {
					attempts++
					if n >= len(tc.attempts) {
						t.Fatalf("attempted unplanned file %d", n)
					}
					if attempts == tc.cancelAt {
						cancel()
					}
					return tc.attempts[n]
				})
			}, nil)
			var got []string
			for _, item := range items {
				got = append(got, item.Status)
			}
			if !slices.Equal(got, tc.want) || tasks.Outcome(err) != tc.taskState ||
				attempts != len(tc.attempts) || err != nil && strings.Contains(err.Error(), "private-path") {
				t.Fatalf("outcome %v, statuses %v, attempts %d", err, got, attempts)
			}
		})
	}
}

func TestModUpdateResultsFromTaskCenter(t *testing.T) {
	a, tt := testDesktop(t)
	a.state.Instances[0].Name, a.state.Instances[1].Name = "同名地图实例", "同名地图实例"
	first, second := a.state.Instances[0], a.state.Instances[1]
	at := time.Date(2026, time.November, 25, 10, 0, 0, 0, time.UTC)
	backupPath := filepath.Join(t.TempDir(), "local-backup.zip")
	stamp := storage.FileStamp{System: runtime.GOOS, File: 1, Size: 1}
	const locate = "定位备份 ZIP"
	rows := []taskRecord{
		{ID: "new", InstanceID: second.ID, Name: second.Name, Action: tasks.ModUpdateAction, Status: taskCanceled, At: at,
			ModUpdateBackup: tasks.LocalZIP{Path: backupPath, Stamp: stamp},
			ModUpdates: tasks.SummarizeModUpdates([]tasks.ModUpdateItem{
				{File: "地图.jar", From: "1.0", To: "2.0", Status: taskCompleted},
				{File: "桥梁.jar.disabled", From: "0.9", To: "1.0", Status: taskFailed},
				{File: "矿灯.jar", To: "2.1", Status: taskCanceled},
				{File: "后续.jar", From: "1.0", To: "2.0", Status: tasks.ModUpdateNotRun},
			})},
		{ID: "old", InstanceID: first.ID, Name: first.Name, Action: tasks.ModUpdateAction, Status: taskFailed, Detail: "旧任务摘要", At: at.Add(-time.Minute)},
	}
	a.store.Root = t.TempDir()
	if err := tasks.Save(a.store.Root, rows); err != nil {
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
				t.Fatal(label, err, tt.Texts())
			}
			tt.Frame()
		}
	}

	click("实例", "1.21.4 · 原版", "查看任务历史")
	for _, label := range []string{"逐项结果 · 实例 #2", "地图.jar", "桥梁.jar.disabled", "矿灯.jar", "后续.jar",
		"1.0 → 2.0", "未知 → 2.1", "已更新", "已失败", "已取消", "未执行"} {
		if !tt.HasText(label) {
			t.Fatal("missing result", label, tt.Texts())
		}
	}
	if len(a.visibleTaskHistory()) != 1 || tt.HasText("旧任务摘要") || tt.HasText(backupPath) {
		t.Fatal("wrong instance history", tt.Texts())
	}
	for _, dark := range []bool{false, true} {
		tt.SetDark(dark)
		tt.Advance(350 * time.Millisecond)
		tt.Frame()
		actions := []string{"返回实例概览"}
		if stamp.Valid() {
			actions = append(actions, locate)
		}
		for _, action := range actions {
			b, ok := tt.Find(action)
			if !ok || b.Y < 42 || b.Y+b.H > 530.5 || b.X+b.W > 820.5 {
				t.Fatal("action not visible", action, b)
			}
		}
		name := "mod-update-results-light"
		if dark {
			name = "mod-update-results-dark"
		}
		capture(t, tt, name)
	}
	a.history[0].ModUpdateBackup.Stamp = storage.FileStamp{}
	tt.Frame()
	if !tt.HasText("备份已保存，但无法确认原 ZIP 的文件身份。请检查保存位置。") || tt.HasText(locate) {
		t.Fatal("unverifiable attachment was offered for location", tt.Texts())
	}
	click("返回实例概览")
	if a.inspected != second.ID || a.detailPage != detailOverview {
		t.Fatal("did not return to the browsed instance")
	}
	click("任务中心", "筛选历史实例", "#1 · 同名地图实例")
	if !tt.HasText("旧任务摘要") || tt.HasText("逐项结果") || tt.HasText(locate) || a.state.Selected != first.ID || a.busy {
		t.Fatal("legacy row gained invented results or changed the launch target", tt.Texts())
	}
}

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	launcherlog "github.com/CAUCraft/LXMCL/internal/log"
	"github.com/egoist/mygo/ui"
)

func TestInstanceLogFileTypeFilter(t *testing.T) {
	a, tt := testDesktop(t)
	a.state.Instances[0].Name, a.state.Instances[1].Name = "同名实例", "同名实例"
	first, second := a.state.Instances[0], a.state.Instances[1]
	at := time.Date(2026, 11, 25, 11, 0, 0, 0, time.Local)
	write := func(root, folder, name string, age time.Duration) string {
		t.Helper()
		dir := filepath.Join(root, folder)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("test fixture"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, at.Add(-age), at.Add(-age)); err != nil {
			t.Fatal(err)
		}
		return path
	}
	latest := write(second.GameDir, "logs", "latest.log", 0)
	newer := write(second.GameDir, "logs", "debug-2.log.gz", time.Hour)
	crash := write(second.GameDir, "crash-reports", "crash-test.txt", 2*time.Hour)
	write(second.GameDir, launcherlog.GameDirectory, "game-test.log", 3*time.Hour)
	write(second.GameDir, "logs", "2026-11-25-1.log.gz", 4*time.Hour)
	debug := write(first.GameDir, "logs", "debug.log", 0)
	entries, err := launcherlog.Recent(t.Context(), second.GameDir)
	if err != nil || len(entries) != 5 {
		t.Fatal("could not list fixture logs", entries, err)
	}
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
	openLogs := func(profile string) {
		t.Helper()
		click(profile, "游戏文件", "日志与崩溃报告…")
	}
	filter := func(label string) {
		t.Helper()
		click("日志文件类型", label)
	}
	click("实例")
	openLogs("1.21.4 · 原版")
	p := a.logLibrary
	a.applyLogFileList(p, entries, nil, a.logRequest, p.fileType)
	a.logText, a.logQuery = "latest fixture\n", "latest"
	tt.Frame()
	if !tt.HasText("全部类型") || a.logPath != latest || p.instance != second {
		t.Fatal("log library did not open unfiltered on the browsed instance", tt.Texts())
	}
	request := a.logRequest
	filter("最新游戏日志")
	if a.logRequest != request || a.logPath != latest || a.logQuery != "latest" || !tt.HasText("latest fixture\n") {
		t.Fatal("matching file lost its preview or content search", tt.Texts())
	}
	click("复制当前日志")
	if tt.Clipboard() != "latest fixture\n" {
		t.Fatal("matching filter copied a different preview")
	}
	tt.Advance(500 * time.Millisecond)
	capture(t, tt, "instance-log-type-latest-light")

	filter("归档日志")
	if a.logPath != "" || a.logText != "" || !tt.HasText("选择日志文件查看预览。") {
		t.Fatal("filtering out the preview did not clear its selection", tt.Texts())
	}
	click("选择实例日志")
	top, topOK := tt.Find("11-25 10:00 · 归档日志 · debug-2.log.gz")
	next, nextOK := tt.Find("11-25 07:00 · 归档日志 · 2026-11-25-1.log.gz")
	if !topOK || !nextOK || top.Y >= next.Y || tt.HasText("11-25 09:00 · 崩溃报告 · crash-test.txt") {
		t.Fatal("archive picker did not keep order and exclude other types", tt.Texts())
	}
	click("11-25 10:00 · 归档日志 · debug-2.log.gz")
	if a.logPath != newer {
		t.Fatal("archive picker did not use the recorded full path")
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	a.logReading, a.logCancel = true, cancel
	request = a.logRequest
	filter("调试日志")
	if ctx.Err() == nil || a.logPath != "" || a.logReading || !tt.HasText("最近的日志列表中没有此类型的文件。") {
		t.Fatal("empty type did not cancel the hidden read and display its empty state", tt.Texts())
	}
	for _, label := range []string{"复制当前日志", "导出预览…", "打开文件"} {
		if el, ok := tt.Access(label); !ok || !el.Disabled {
			t.Fatalf("log action %s remained available without a selected file", label)
		}
	}
	a.applyLogFileList(p, entries, nil, request, archiveLogFile)
	tt.SetDark(true)
	tt.Advance(500 * time.Millisecond)
	tt.Frame()
	if a.logPath != "" || !tt.HasText("最近的日志列表中没有此类型的文件。") {
		t.Fatal("list refresh reopened a hidden file")
	}
	capture(t, tt, "instance-log-type-empty-dark")
	filter("全部类型")
	click("选择实例日志", "11-25 09:00 · 崩溃报告 · crash-test.txt")
	if a.logPath != crash {
		t.Fatal("clearing the type did not restore the report")
	}

	filter("启动日志")
	click("选择实例日志", "11-25 08:00 · 启动日志 · game-test.log")
	if a.logPath != filepath.Join(second.GameDir, launcherlog.GameDirectory, "game-test.log") {
		t.Fatal("launcher filter selected an unrelated file")
	}
	tt.Key(0, ui.KeyEscape)
	openLogs("1.21.1 · 原版 · 启动目标")
	other := a.logLibrary
	otherEntries, err := launcherlog.Recent(t.Context(), first.GameDir)
	if err != nil {
		t.Fatal(err)
	}
	a.applyLogFileList(other, otherEntries, nil, a.logRequest, other.fileType)
	tt.Frame()
	if other == p || other.fileType != "" || a.logPath != debug || !tt.HasText("全部类型") || a.state.Selected != first.ID {
		t.Fatal("switching instances reused the previous file type or changed the launch target", tt.Texts())
	}
}

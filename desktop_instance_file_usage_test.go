package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/egoist/mygo/ui"
)

func TestInstanceFileUsageFromOverview(t *testing.T) {
	a, tt := testDesktop(t)
	a.ctx = t.Context()
	a.state.Instances[0].Name, a.state.Instances[1].Name = "同名实例", "同名实例"
	first, second := a.state.Instances[0], a.state.Instances[1]
	a.game.Cache = filepath.Join(second.GameDir, "cache")
	if err := os.Mkdir(a.game.Cache, 0700); err != nil {
		t.Fatal(err)
	}
	write := func(dir, data string) {
		if err := os.WriteFile(filepath.Join(dir, "synthetic"), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(second.GameDir, "1234")
	write(a.game.Cache, "excluded")

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
	wait := func(label string) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for !tt.HasText(label) && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
			tt.Frame()
		}
		if !tt.HasText(label) {
			t.Fatalf("missing %s (%v)", label, tt.Texts())
		}
	}
	const start = "查看文件用量"
	click("实例", "1.21.4 · 原版")
	if a.instanceUsage != nil || a.inspected != second.ID || a.state.Selected != first.ID {
		t.Fatal("browsing the second instance started a scan or changed the launch target")
	}
	click(start)
	wait("1 个文件 · 4 B（大小合计）")
	wait("可释放空间取决于文件系统。")
	tt.Advance(500 * time.Millisecond)
	capture(t, tt, "instance-file-usage-result-light")

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	late := make(chan instanceUsageResult, 1)
	a.instanceUsage = &instanceUsageRequest{target: second, cancel: cancel, done: late}
	tt.Frame()
	click("取消统计")
	if ctx.Err() == nil || !tt.HasText("已取消统计") {
		t.Fatal("cancel did not stop the request and display the canceled state")
	}
	late <- instanceUsageResult{stats: storage.FileUsage{Files: 99, Bytes: 99}}
	tt.Frame()
	if !tt.HasText("已取消统计") || tt.HasText("99 个文件 · 99 B（大小合计）") {
		t.Fatal("a canceled result replaced its feedback")
	}

	ctx, cancel = context.WithCancel(t.Context())
	defer cancel()
	late = make(chan instanceUsageResult, 1)
	a.instanceUsage = &instanceUsageRequest{target: second, cancel: cancel, done: late}
	tt.Frame()
	click("1.21.1 · 原版 · 启动目标")
	late <- instanceUsageResult{stats: storage.FileUsage{Files: 99, Bytes: 99}}
	if ctx.Err() == nil || a.instanceUsage != nil || !tt.HasText(start) {
		t.Fatal("switching instances did not discard the previous scan", tt.Texts())
	}
	click(start)
	wait("0 个文件 · 0 B（大小合计）")

	// Change the confirmed instance directory to an unavailable path.
	a.state.Instances[0].GameDir = filepath.Join(first.GameDir, "absent")
	tt.Frame()
	click(start)
	wait("无法完整统计，请检查目录和访问权限后重试。")
	tt.SetDark(true)
	tt.Advance(500 * time.Millisecond)
	capture(t, tt, "instance-file-usage-unavailable-dark")
	if a.state.Selected != first.ID || a.busy || a.task.Stage != "" {
		t.Fatal("checking usage changed the launch target or started a task")
	}
}

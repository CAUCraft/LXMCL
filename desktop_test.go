package main

import (
	"context"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	launcherlog "github.com/CAUCraft/LXMCL/internal/log"
	"github.com/CAUCraft/LXMCL/internal/saves"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/display"
)

func TestMicrosoftLoginDialogUsesSystemBrowserAndCancels(t *testing.T) {
	a, tt := testDesktop(t)
	if err := tt.Click("账号中心"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Microsoft 账号"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("登录 Microsoft"); !ok {
		t.Fatal("Microsoft sign-in is missing from account settings")
	}
	canceled := false
	const verification = "https://www.microsoft.com/devicelogin"
	a.microsoft = &microsoftLoginView{open: true, cancel: func() { canceled = true },
		code: launcher.MicrosoftDeviceCode{Code: "TEST-CODE", VerificationURL: verification}}
	tt.Frame()
	capture(t, tt, "microsoft-device")
	if err := tt.Click("复制登录代码"); err != nil {
		t.Fatal(err)
	}
	if tt.Clipboard() != "TEST-CODE" {
		t.Fatal("the copy action did not copy the device code")
	}
	if err := tt.Click("打开 Microsoft 登录页"); err != nil {
		t.Fatal(err)
	}
	if opened := tt.OpenedURLs(); len(opened) != 1 || opened[0] != verification {
		t.Fatalf("expected the official verification page, got %v", opened)
	}
	tt.Key(0, ui.KeyEscape)
	if a.microsoft != nil || !canceled || a.state.MicrosoftProfile != "" {
		t.Fatal("closing sign-in did not cancel the pending login")
	}
	// OAuth can finish while another write owns the captured configuration.
	// Retain its result for a save retry instead of starting a concurrent write.
	p := &microsoftLoginView{open: true}
	a.microsoft, a.busy = p, true
	verified := launcher.MicrosoftAccount{Profile: launcher.MicrosoftProfile{ID: "0123456789abcdef0123456789abcdef", Name: "Player"}}
	a.saveMicrosoftLogin(p, verified, launcher.MinecraftSession{})
	tt.Frame()
	if p.account != verified || p.saving || !a.busy || a.state.MicrosoftProfile != "" {
		t.Fatal("completed OAuth changed state during another operation")
	}
	if _, ok := tt.Find("重试保存登录"); !ok {
		t.Fatal("verified sign-in cannot retry saving")
	}
}

func TestTaskRetryStaysWithCapturedAccount(t *testing.T) {
	a, _ := testDesktop(t)
	i := a.state.Instances[0]
	row := taskRecord{InstanceID: i.ID, Revision: i.Revision, Version: i.Version,
		Action: taskLaunch, Status: taskFailed, Account: a.state.AccountKey()}
	if _, ok := launcher.RetryTarget(row, a.state); !ok {
		t.Fatal("same-account launch could not be retried")
	}
	a.state.MicrosoftProfile = "0123456789abcdef0123456789abcdef"
	if _, ok := launcher.RetryTarget(row, a.state); ok {
		t.Fatal("the previous offline task can be retried with Microsoft credentials")
	}
	row.Account = a.state.AccountKey()
	if err := a.state.UseOfflinePlayer("OtherPlayer"); err != nil {
		t.Fatal(err)
	}
	if _, ok := launcher.RetryTarget(row, a.state); ok {
		t.Fatal("Microsoft launch can be retried under an offline player")
	}
	a.state.UseStation(launcher.StationProfile{Key: "station-fixture"})
	row.Account = ""
	if _, ok := launcher.RetryTarget(row, a.state); ok {
		t.Fatal("a legacy launch without a captured account can be retried under a station identity")
	}
	row.Account = a.state.AccountKey()
	if _, ok := launcher.RetryTarget(row, a.state); !ok {
		t.Fatal("retry did not retain the captured station account")
	}
}

func TestDesktopNavigationKeepsLaunchTarget(t *testing.T) {
	a, tt := testDesktop(t)
	capture(t, tt, "home")
	tt.Key(0, ui.KeyF1)
	mods, key, _ := display.HotkeyBinding(searchHotkey, display.HotkeyAuto)
	tt.Key(mods, key)
	capture(t, tt, "instances")
	tt.Type("建筑")
	if a.page != pageLibrary || a.query != "建筑" {
		t.Fatalf("global search did not focus the instance query: page %d, query %q", a.page, a.query)
	}
	if _, ok := tt.Find("生存世界"); ok {
		t.Fatal("search did not hide the unmatched instance")
	}
	if err := tt.Click("建筑测试"); err != nil {
		t.Fatal(err)
	}
	if a.inspected != 2 || a.state.Selected != 1 {
		t.Fatalf("browse target %d, launch target %d", a.inspected, a.state.Selected)
	}
	capture(t, tt, "instance-detail")
	if err := tt.Click("更多实例操作"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("重命名"); !ok {
		t.Fatal("rename action is missing from the instance menu")
	}
	capture(t, tt, "instance-menu")
	tt.Key(0, ui.KeyEscape)
	if a.inspected != 2 {
		t.Fatal("Escape should dismiss the menu before leaving the instance")
	}
	tt.Key(0, ui.KeyF2)
	if !a.form.open || a.form.target.ID != 2 {
		t.Fatalf("F2 did not open the viewed instance: %+v", a.form)
	}
	capture(t, tt, "rename")
	tt.Key(0, ui.KeyEscape)
	if a.form.open || a.inspected != 2 {
		t.Fatalf("first Escape should only close the editor; inspected = %d", a.inspected)
	}
	tt.Key(0, ui.KeyEscape)
	if a.inspected != 0 || a.state.Selected != 1 {
		t.Fatalf("second Escape should return to the list; inspected = %d, selected = %d", a.inspected, a.state.Selected)
	}
	a.state.Instances = a.state.Instances[:1]
	a.inspect(a.state.Instances[0])
	a.query = "不存在的实例"
	tt.SetSize(820, 530)
	tt.Frame()
	capture(t, tt, "instance-filter-empty-compact")
	if err := tt.Click("清除筛选"); err != nil || a.query != "" || a.state.Selected != 1 {
		t.Fatalf("cannot clear a search from single-instance details: %v", err)
	}
	a.state.Instances, a.state.Selected, a.page = nil, 0, pageHome
	tt.SetSize(820, 530)
	tt.Frame()
	capture(t, tt, "home-first-run")
	if err := tt.Click("切换启动账号"); err != nil || a.page != pageAccount {
		t.Fatalf("first-run account setup is unreachable: %v", err)
	}
	tt.Key(0, ui.KeyEscape)
	if err := tt.Click("安装实例…"); err != nil || a.downloadPage != downloadGame || a.page != pageDownloads {
		t.Fatalf("first-run install is unreachable: %v", err)
	}
}

func TestDesktopTasksReachableDuringDownload(t *testing.T) {
	a, tt := testDesktop(t)
	a.busy, a.taskName = true, "建筑测试"
	a.task = launcher.Progress{Stage: "下载资源", Done: 8, Total: 24}
	cancels := 0
	a.cancelTask = func() { cancels++ }
	a.history = []taskRecord{{
		InstanceID: 1,
		Name:       "生存世界", Version: "1.21.1", Action: "启动游戏", Status: "已失败",
		Detail: "请检查 Java 设置后重试。", At: time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC),
	}}
	tt.SetSize(820, 530)
	tt.SetDark(true)
	tt.Advance(500 * time.Millisecond)
	tt.Frame()
	capture(t, tt, "home-dark-compact")
	if err := tt.Click("查看任务"); err != nil {
		t.Fatal(err)
	}
	if a.page != pageDownloads || a.downloadPage != downloadTasks || a.state.Selected != 1 {
		t.Fatalf("unexpected task navigation: page %d, tab %d, target %d", a.page, a.downloadPage, a.state.Selected)
	}
	if _, ok := tt.Find("历史记录"); !ok {
		t.Fatal("task history was not rendered")
	}
	capture(t, tt, "tasks-dark-compact")
	if err := tt.Click("复制错误详情"); err != nil || tt.Clipboard() != a.history[0].Detail {
		t.Fatalf("failed to copy the task error: %v", err)
	}
	if err := tt.Click("取消任务"); err != nil {
		t.Fatal(err)
	}
	if !a.taskCanceling || cancels != 1 {
		t.Fatal("cancel did not retain the pending task and show immediate feedback")
	}
	if err := tt.Click("正在取消…"); err != nil || cancels != 1 || !a.busy {
		t.Fatalf("cancel could be submitted again or cleared the active operation: %v", err)
	}
	capture(t, tt, "tasks-canceling-compact")
	const failure = "[12:00:01] [main/ERROR] Failed to load mod\njava.lang.RuntimeException: demo\n\tat demo.Client.start(Client.java:42)\n"
	a.logOpen, a.logPath, a.logText = true, "launcher.log", "[12:00:00] [main/INFO] Minecraft is starting.\n"+failure
	tt.Frame()
	if _, ok := tt.Find("游戏日志"); !ok {
		t.Fatal("the log viewer did not open")
	}
	capture(t, tt, "log-dark-compact")
	if err := tt.Click("警告 / 错误"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("复制当前日志"); err != nil || tt.Clipboard() != failure {
		t.Fatalf("log filter lost the stack trace: %v", err)
	}
	mods, key, _ := display.HotkeyBinding("mod+f", display.HotkeyAuto)
	tt.Key(mods, key)
	tt.Type("Client.java")
	if a.logQuery != "Client.java" {
		t.Fatal("find shortcut did not focus the log search")
	}
	capture(t, tt, "log-filtered-compact")
	if err := tt.Click("复制当前日志"); err != nil || tt.Clipboard() != failure {
		t.Fatalf("log search lost the event context: %v", err)
	}
	a.logExportErr = "无法写入所选文件：文件被其他程序占用，请关闭占用程序或选择其他保存位置。"
	tt.Frame()
	capture(t, tt, "log-export-error-compact")
	if _, ok := tt.Find("导出预览…"); !ok {
		t.Fatal("failed export hid the retry action")
	}
	if err := tt.Click("全部日志"); err != nil || a.logExportErr != "" {
		t.Fatalf("a changed preview retained the previous export error: %v", err)
	}
	a.logText = "[new/INFO] reloaded Client.java\n"
	tt.Frame()
	if err := tt.Click("复制当前日志"); err != nil || tt.Clipboard() != a.logText {
		t.Fatalf("copy used the previous filtered preview after refresh: %v", err)
	}
	if err := tt.Click("警告 / 错误"); err != nil {
		t.Fatal(err)
	}
	for _, dark := range []bool{false, true} {
		tt.SetDark(dark)
		tt.Advance(time.Second)
		tt.Frame()
		name := "log-no-match-light-compact"
		if dark {
			name = "log-no-match-dark-compact"
		}
		capture(t, tt, name)
	}
	if err := tt.Click("清除筛选"); err != nil || a.logQuery != "" || a.logFilter != 0 {
		t.Fatalf("cannot recover from combined log filters: %v", err)
	}
	tt.Type("reloaded")
	if a.logQuery != "reloaded" {
		t.Fatal("search lost keyboard focus after clearing filters")
	}
	if err := tt.Click("复制当前日志"); err != nil || tt.Clipboard() != a.logText || a.state.Selected != 1 {
		t.Fatalf("reset did not restore the current log in place: %v", err)
	}
	tt.Key(0, ui.KeyEscape)
	if a.logOpen {
		t.Fatal("Escape did not close the log viewer")
	}
	a.busy = false
	a.task = launcher.Progress{}
	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("浏览…"); !ok {
		t.Fatal("native game-directory picker has no entry point")
	}
	capture(t, tt, "settings-dark-compact")
}

func TestCompactTitleBarCentersNavigationWithWindowsControls(t *testing.T) {
	a, tt := testDesktop(t)
	tt.SetSize(820, 530)
	a.ready, a.loadErr = false, "无法读取 launcher.json"
	tt.Frame()
	if _, ok := tt.Find("主导航"); ok {
		t.Fatal("unavailable navigation offered before startup")
	}
	if err := tt.Click("复制错误详情"); err != nil || tt.Clipboard() != a.loadErr {
		t.Fatalf("cannot copy startup failure: %v", err)
	}
	if _, ok := tt.Find("重新读取配置"); !ok {
		t.Fatal("missing startup recovery action")
	}
	capture(t, tt, "startup-config-recovery")
	a.ready, a.loadErr = true, ""
	for _, width := range []int{820, 1140} {
		tt.SetSize(width, 580)
		tt.Frame()
		bounds, ok := tt.Find("主导航")
		if !ok {
			t.Fatalf("navigation missing at %d DIP", width)
		}
		middle := bounds.X + bounds.W/2
		if math.Abs(float64(middle)-float64(width)/2) > 2 {
			t.Fatalf("navigation center %.1f is not at window center %.1f", middle, float64(width)/2)
		}
	}
}

func TestCompactSettingsAndLicenses(t *testing.T) {
	a, tt := testDesktop(t)
	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("应用设置"); ok {
		t.Fatal("save actions should only appear when there are pending changes")
	}
	if err := tt.Click("Java"); err != nil {
		t.Fatal(err)
	}
	tt.SetSize(820, 530)
	a.draft.JavaPath, a.javaError, a.javaInfo = "missing-java", true, "找不到 Java，请重新选择路径。"
	tt.Frame()
	capture(t, tt, "global-java-recovery")
	if err := tt.Click("自动"); err != nil || a.draft.JavaPath != "" || tt.HasText("Java 检测失败") {
		t.Fatalf("automatic selection kept the failed Java path: %v", err)
	}
	if err := tt.Click("游戏"); err != nil {
		t.Fatal(err)
	}
	a.memory = "6144"
	tt.Frame()
	if err := tt.Click("还原更改"); err != nil {
		t.Fatal(err)
	}
	if a.memory != "4096" {
		t.Fatalf("restored memory = %q", a.memory)
	}
	tt.SetSize(820, 530)
	a.memory = "4GiB"
	tt.Frame()
	capture(t, tt, "settings-invalid-compact")
	if _, ok := tt.Find("最大内存须为 1024–65536 MiB 的整数"); !ok {
		t.Fatal("invalid memory has no inline explanation")
	}
	if err := tt.Click("应用设置"); err != nil || a.busy || a.state.Settings.Memory != 4096 {
		t.Fatalf("invalid memory was submitted: %v", err)
	}
	tt.Key(0, ui.KeyF1)
	if _, ok := tt.Find("有未保存的更改"); !ok || a.memory != "4GiB" {
		t.Fatal("changing settings tabs hid or discarded pending changes")
	}
	if err := tt.Click("还原更改"); err != nil || a.memory != "4096" {
		t.Fatalf("cannot restore settings from the about tab: %v", err)
	}
	capture(t, tt, "about-compact")
	if err := tt.Click("查看许可证"); err != nil {
		t.Fatal(err)
	}
	if !a.licensesOpen || a.licenseFile != "LICENSE" {
		t.Fatal("licenses did not open on the project license")
	}
	capture(t, tt, "licenses")
	tt.Key(0, ui.KeyEscape)
	if a.licensesOpen {
		t.Fatal("Escape did not dismiss the licenses")
	}
}

func TestInstanceFilterAndVersionEditor(t *testing.T) {
	a, tt := testDesktop(t)
	a.releases = []launcher.Version{{ID: "1.21.4"}, {ID: "1.21.1"}}
	if err := tt.Click("实例"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("待下载"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("生存世界"); ok {
		t.Fatal("cached instance appeared in the download filter")
	}
	capture(t, tt, "instance-filter")
	if err := tt.Click("建筑测试"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("更换版本"); err != nil {
		t.Fatal(err)
	}
	if !a.form.open || a.form.target.ID != 2 || a.form.version != "1.21.4" || a.state.Selected != 1 {
		t.Fatalf("version editor bound to the wrong instance: %+v", a.form)
	}
	capture(t, tt, "instance-version")
	a.form.version = "1.20.1"
	tt.Frame()
	if err := tt.Click("保存版本"); err != nil {
		t.Fatal(err)
	}
	if !a.form.open || a.form.err == "" || a.state.Instances[1].Version != "1.21.4" {
		t.Fatal("an unsupported version was accepted")
	}
	tt.Key(0, ui.KeyEscape)
	a.query = "1.21"
	for _, label := range []string{"主页", "实例设置"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if a.instanceFilter != 0 || a.query != "1.21" || a.detailPage != detailLaunch || a.inspected != 1 || a.state.Selected != 1 {
		t.Fatal("home settings did not reveal the target while keeping its matching search")
	}
}

func TestRecoveredHistoryKeepsCurrentGameLog(t *testing.T) {
	a, tt := testDesktop(t)
	current, former := a.state.Instances[1], a.state.Instances[0]
	activeLog := filepath.Join(current.GameDir, "launcher-logs", "game-current.log")
	oldLog := filepath.Join(former.GameDir, "launcher-logs", "game-previous.log")
	a.running = &launcher.Running{PID: 123, Log: activeLog}
	a.runningID, a.lastLog = current.ID, activeLog
	a.restoreTaskHistory([]taskRecord{{Name: former.Name, InstanceID: former.ID, Log: oldLog, Status: taskExited}})
	tt.SetSize(820, 530)
	if err := tt.Click("任务中心"); err != nil {
		t.Fatal(err)
	}
	for _, dark := range []bool{false, true} {
		tt.SetDark(dark)
		tt.Advance(time.Second)
		tt.Frame()
		name := "running-game-history-light"
		if dark {
			name = "running-game-history-dark"
		}
		capture(t, tt, name)
	}
	if err := tt.Click("查看当前日志"); err != nil {
		t.Fatal(err)
	}
	if a.logPath != activeLog || a.lastLog != activeLog || a.history[0].Log != oldLog || a.state.Selected != former.ID {
		t.Fatal("history recovery changed the running game's log or the home selection")
	}
}

func TestCanceledTaskHistoryKeepsAdditionalDiagnostics(t *testing.T) {
	a, tt := testDesktop(t)
	a.openTasks()
	row := taskRecord{ID: "canceled-export", Name: "实例", Action: "导出整合包", Status: taskCanceled, Detail: context.Canceled.Error()}
	a.history = []taskRecord{row}
	tt.Frame()
	if _, ok := tt.Find("复制错误详情"); ok || tt.HasText(row.Detail) {
		t.Fatal("plain cancellation presented as a diagnostic")
	}
	a.history[0].Detail += "\n清理临时文件失败"
	tt.Frame()
	if err := tt.Click("复制错误详情"); err != nil || tt.Clipboard() != a.history[0].Detail {
		t.Fatal("additional cancellation diagnostics could not be copied")
	}
}

func TestTaskCenterFiltersAndDisablesStaleRetry(t *testing.T) {
	a, tt := testDesktop(t)
	i := a.state.Instances[1]
	a.history = []taskRecord{
		{InstanceID: i.ID, Revision: i.Revision, Name: i.Name, Version: i.Version, Action: taskVerify, Status: taskFailed, Detail: "文件摘要不匹配，请重新校验。", At: time.Now()},
		{Name: "已完成的任务", Version: "1.21.1", Action: "下载资源", Status: taskCompleted, At: time.Now().Add(-time.Minute),
			Resource: &resourceTask{Title: "高清资源包"}},
	}
	a.history[0].Log = filepath.Join(i.GameDir, "launcher-logs", "game-session.log")
	a.lastLog = a.history[0].Log
	if err := tt.Click("任务中心"); err != nil {
		t.Fatal(err)
	}
	if a.page != pageDownloads || a.downloadPage != downloadTasks {
		t.Fatal("title bar action did not open the task center")
	}
	if err := tt.Click("主页"); err != nil {
		t.Fatal(err)
	}
	mods, key, _ := display.HotkeyBinding(tasksHotkey, display.HotkeyAuto)
	tt.Key(mods, key)
	if a.page != pageDownloads || a.downloadPage != downloadTasks || a.state.Selected != 1 {
		t.Fatal("task shortcut did not preserve the launch selection")
	}
	tt.SetSize(820, 530)
	tt.Frame()
	completed := a.history[1]
	for _, label := range []string{"选择历史任务", completed.Name + " · " + completed.Action + " · " + completed.Status + " · " + completed.At.Local().Format("01-02 15:04")} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := tt.Find("资源 · 高清资源包"); !ok || a.browsedTask != taskKey(completed) {
		t.Fatal("compact task selection lost the resource or selected record")
	}
	if _, ok := tt.Find("打开日志目录"); ok {
		t.Fatal("task without a log inherited another task's folder")
	}
	tt.SetSize(1140, 720)
	tt.Frame()
	a.notice, a.noticeError, a.noticeTaskKey = "任务失败\n"+strings.Repeat("文件读取失败\n", 12), true, taskKey(a.history[0])
	tt.Frame()
	if err := tt.Click("复制通知详情"); err != nil || tt.Clipboard() != a.notice {
		t.Fatal("full notification diagnostic could not be copied")
	}
	capture(t, tt, "task-notice-long-error")
	if err := tt.Click("查看任务"); err != nil || a.browsedTask != taskKey(a.history[0]) || a.state.Selected != 1 {
		t.Fatalf("task notice did not focus the failed record: %v", err)
	}
	if err := tt.Click("失败 / 取消"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("已完成的任务"); ok {
		t.Fatal("completed task appeared in the failed filter")
	}
	if _, ok := tt.Find("重试"); !ok {
		t.Fatal("unchanged task target cannot be retried")
	}
	a.browsedTask = a.history[1].Key()
	a.notice, a.noticeTaskKey = "校验失败", a.history[0].Key()
	if err := tt.Click("主页"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("查看任务"); err != nil || a.browsedTask != a.history[0].Key() {
		t.Fatalf("notification opened the wrong task: %v", err)
	}
	if err := tt.Click("复制错误详情"); err != nil || tt.Clipboard() != a.history[0].Detail {
		t.Fatalf("copy error detail: %q, %v", tt.Clipboard(), err)
	}
	tt.Advance(300 * time.Millisecond)
	capture(t, tt, "task-filter")
	a.state.Instances[1].Revision++
	tt.Frame()
	if _, ok := tt.Find("重试"); ok {
		t.Fatal("a task for an obsolete revision can still be retried")
	}
	if _, ok := tt.Find("实例配置、登录账号或资源信息已变化，请检查后重新发起。"); !ok {
		t.Fatal("unavailable retry has no explanation")
	}
	if err := tt.Click("查看实例"); err != nil {
		t.Fatal(err)
	}
	if a.inspected != i.ID || a.detailPage != detailLaunch || a.state.Selected != 1 {
		t.Fatal("task recovery did not open the captured instance settings without changing the launch target")
	}
	a.history = []taskRecord{{InstanceID: i.ID, Revision: i.Revision, Name: i.Name, Action: "恢复存档 · 1 个世界", Status: taskCanceled}}
	a.openTasks()
	tt.Frame()
	if err := tt.Click("查看实例"); err != nil || a.detailPage != detailFiles || a.inspected != i.ID || a.state.Selected != 1 {
		t.Fatalf("restore history did not return to the target's files: %v", err)
	}
	a.state.Instances[1].Loader, a.state.Instances[1].LoaderVersion = "Fabric", "0.16.14"
	a.history = []taskRecord{{InstanceID: i.ID, Name: i.Name, Version: i.Version, Action: "导入 2 项模组", Status: taskFailed,
		At: time.Now(), Detail: "已导入 1 / 2 项：notes.txt: 资源文件名或大小不受支持", Resource: &resourceTask{Kind: string(launcher.Mods)}}}
	a.openTasks()
	tt.SetSize(820, 530)
	tt.SetDark(true)
	tt.Frame()
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	if _, ok := tt.Find("重试"); ok {
		t.Fatal("local files cannot be replayed as a download")
	}
	capture(t, tt, "local-resource-task-compact")
	if err := tt.Click("查看实例"); err != nil || a.content.kind() != launcher.Mods || a.inspected != i.ID || a.state.Selected != 1 {
		t.Fatalf("local task did not reopen the original mod list: %v", err)
	}

}

func TestInstanceLaunchModesRetainCustomDraft(t *testing.T) {
	a, tt := testDesktop(t)
	target := &a.state.Instances[1]
	target.Launch = launcher.LaunchOptions{
		JavaMode: launcher.JavaCustom, JavaPath: filepath.Join(t.TempDir(), "bin", "java"),
		Display: launcher.DisplayResolution, Width: 1920, Height: 1080,
	}
	for _, label := range []string{
		"实例", target.Name, "启动设置",
		"自定义分辨率", "启动时全屏", "启动时全屏", "自定义分辨率",
		"指定路径", "自动选择", "自动选择", "指定路径",
	} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	fields := a.launchDrafts[target.ID].fields
	if got, err := fields.options(); err != nil || got != target.Launch {
		t.Fatalf("switching modes changed custom settings: %+v, %v", got, err)
	}
	if _, ok := tt.Find("应用并启动"); ok {
		t.Fatal("returning to the saved modes left a pending change")
	}

	// Hidden edits stay in the draft until the user applies or reverts it.
	fields.display, fields.javaMode = launcher.DisplayFullscreen, launcher.JavaAuto
	saved := launcher.LaunchOptions{Display: launcher.DisplayFullscreen, JavaMode: launcher.JavaAuto}
	if fields.changed(saved) {
		t.Fatal("hidden custom values marked unchanged settings as edited")
	}
	if got, err := fields.options(); err != nil || got != saved {
		t.Fatalf("inactive custom values would be saved: %+v, %v", got, err)
	}
}

func TestInheritedJavaOpensGlobalSettings(t *testing.T) {
	a, tt := testDesktop(t)
	a.state.Settings.JavaPath = filepath.Join(t.TempDir(), "java")
	a.draft = a.state.Settings
	i := a.state.Instances[1]
	a.inspect(i)
	a.detailPage = detailLaunch
	tt.SetSize(820, 530)
	tt.Frame()
	if err := tt.Click("修改全局 Java"); err != nil {
		t.Fatal(err)
	}
	if a.page != pageSettings || a.settingsPage != 1 || a.state.Selected != 1 || a.state.Instances[1] != i {
		t.Fatal("opening inherited Java settings changed the instance or launch target")
	}
}

func TestInstanceSettingsKeepDraftAndLaunchTargetSeparate(t *testing.T) {
	a, tt := testDesktop(t)
	tt.SetSize(820, 530)
	tt.SetDark(true)
	tt.Frame()
	for _, name := range []string{"实例", "建筑测试", "启动设置", "8 GiB"} {
		if err := tt.Click(name); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := tt.Find("应用并启动"); !ok {
		t.Fatal("pending settings have no apply-and-launch action")
	}
	if a.state.Selected != 1 || a.state.Instances[1].Launch.Memory != 0 {
		t.Fatal("editing an instance changed the launch target or applied the draft")
	}
	capture(t, tt, "instance-launch-dark")
	if err := tt.Click("游戏文件"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("打开存档目录"); !ok {
		t.Fatal("game files has no saves shortcut")
	}
	capture(t, tt, "instance-files-dark")
	if err := tt.Click("启动设置"); err != nil {
		t.Fatal(err)
	}
	if a.launchDrafts[2].fields.memory != "8192" {
		t.Fatal("switching tabs lost the instance draft")
	}
	a.launchDrafts[2].fields.memory = "8g"
	tt.Frame()
	if _, ok := tt.Find("最大内存须为 1024–65536 MiB，留空则跟随全局"); !ok {
		t.Fatal("invalid instance memory has no explanation before applying")
	}
	if err := tt.Click("应用并启动"); err != nil || a.busy || a.launchDrafts[2].err != "" {
		t.Fatalf("invalid draft reached submit: %v", err)
	}
	tt.Advance(250 * time.Millisecond)
	tt.Frame()
	capture(t, tt, "instance-draft-invalid")
	if err := tt.Click("游戏文件"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("检查启动设置"); err != nil || a.detailPage != detailLaunch || a.state.Selected != 1 {
		t.Fatalf("cannot correct the draft from another tab: %v", err)
	}
	if err := tt.Click("还原更改"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("应用并启动"); ok {
		t.Fatal("reverting the draft left an apply-and-launch action")
	}
	tt.Key(0, ui.KeyEscape)
	if a.detailPage != detailOverview || a.inspected != 2 {
		t.Fatal("Escape did not return to the instance overview")
	}
	if err := tt.Click("更多实例操作"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("复制实例…"); err != nil {
		t.Fatal(err)
	}
	if a.form.target.ID != 2 || a.form.mode != "duplicate" || a.state.Selected != 1 {
		t.Fatal("copy dialog was not bound to the viewed instance")
	}
	if a.form.copyContent || a.form.name != "建筑测试 副本" {
		t.Fatalf("unexpected copy defaults: %+v", a.form)
	}
	capture(t, tt, "duplicate-compact")
	if err := tt.Click("同时复制资源和游戏设置"); err != nil || !a.form.copyContent {
		t.Fatalf("copying files is not selectable: %v", err)
	}
	tt.Advance(250 * time.Millisecond)
	tt.Frame()
	capture(t, tt, "duplicate-content-compact")
	tt.SetDark(false)
	tt.SetSize(1140, 720)
	tt.Frame()
	tt.Advance(250 * time.Millisecond)
	tt.Frame()
	capture(t, tt, "duplicate-content-light")
	tt.Key(0, ui.KeyEscape)
	if a.form.open || len(a.state.Instances) != 2 || a.state.Selected != 1 {
		t.Fatal("canceling a copy changed the instance list or launch target")
	}

}

func TestSaveRestorePreviewKeepsTarget(t *testing.T) {
	a, tt := testDesktop(t)
	target := a.state.Instances[1]
	a.inspect(target)
	a.detailPage = detailFiles
	stamp := time.Date(2026, 11, 18, 7, 30, 0, 0, time.Local)
	a.backups = &backupView{instanceID: target.ID, entries: []saves.BackupEntry{
		{Name: "社团世界备份.zip", Size: 138 << 20, Modified: stamp},
		{Name: "活动前备份.zip", Size: 89 << 20, Modified: stamp.Add(-24 * time.Hour)},
		{Name: "初始地图.zip", Size: 8192, Modified: stamp.Add(-48 * time.Hour)},
	}}
	tt.Frame()
	capture(t, tt, "instance-backups-light")
	tt.SetSize(820, 530)
	tt.SetDark(true)
	tt.Frame()
	tt.Advance(250 * time.Millisecond)
	tt.Frame()
	capture(t, tt, "instance-backups-compact")
	// Keep the asynchronous file reader outside the UI fixture.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	a.ctx = ctx
	if err := tt.Click("预览备份 社团世界备份.zip"); err != nil {
		t.Fatal(err)
	}
	p := a.restore
	if p == nil || p.target.ID != target.ID || p.filename != "社团世界备份.zip" || a.state.Selected != 1 {
		t.Fatal("recent preview opened a different backup or changed the home target")
	}
	p.loading, p.worlds = false, []string{"建筑测试", "红石实验", "社团生存"}
	tt.SetSize(1140, 720)
	tt.SetDark(false)
	tt.Frame()
	tt.Advance(250 * time.Millisecond)
	tt.Frame()
	capture(t, tt, "restore-worlds-light")
	if err := tt.Click("红石实验"); err != nil || p.selected != 2 {
		t.Fatalf("world selection: %d %v", p.selected, err)
	}
	tt.SetSize(820, 530)
	tt.SetDark(true)
	tt.Frame()
	tt.Advance(250 * time.Millisecond)
	tt.Frame()
	capture(t, tt, "restore-selected-compact")
	tt.Key(0, ui.KeyF2)
	if a.form.open || a.state.Selected != 1 {
		t.Fatal("preview leaked a global shortcut or changed launch target")
	}
	a.state.Instances[1].Revision++
	tt.Frame()
	if _, ok := tt.Find("恢复世界"); ok || a.busy {
		t.Fatal("stale restore can still be submitted")
	}
	if _, ok := tt.Find("返回实例"); !ok {
		t.Fatal("stale target has no recovery action")
	}
	capture(t, tt, "restore-error-compact")
	tt.Key(0, ui.KeyEscape)
	if a.restore != nil || a.detailPage != detailFiles || a.inspected != target.ID {
		t.Fatal("Escape left the originating files tab")
	}
}

func testDesktop(t *testing.T) (*desktop, *ui.Tester) {
	t.Helper()
	logo, err := ui.DecodeImage(logoPNG)
	if err != nil {
		t.Fatal(err)
	}
	hero, err := ui.DecodeImage(homePNG)
	if err != nil {
		t.Fatal(err)
	}
	a := &desktop{
		logo: logo, hero: hero, ready: true, installed: map[int]bool{1: true},
		state: launcher.State{Selected: 1, NextID: 3, Settings: launcher.Settings{Player: "Player", Memory: 4096, GameRoot: t.TempDir()},
			Instances: []launcher.Instance{
				{ID: 1, Name: "生存世界", Version: "1.21.1", Loader: "Vanilla", GameDir: t.TempDir()},
				{ID: 2, Name: "建筑测试", Version: "1.21.4", Loader: "Vanilla", GameDir: t.TempDir()},
			}},
	}
	a.draft, a.memory = a.state.Settings, "4096"
	tt := ui.NewTesterWith(a.view, 1140, 720, ui.TesterOptions{Preferences: ui.Preferences{ReduceMotion: true}})
	tt.SetTitleBar(ui.TitleBar{Height: titleBarHeight, Right: 138})
	tt.Frame()
	return a, tt
}

func TestResourceAndServerEntrypoints(t *testing.T) {
	a, tt := testDesktop(t)
	a.state.Instances[1].Loader, a.state.Instances[1].LoaderVersion = "Fabric", "0.16.14"
	i := a.state.Instances[1]
	a.content = contentView{id: i.ID, revision: i.Revision, tab: 1, files: []launcher.ContentFile{
		{Name: "demo.jar", Enabled: true, Size: 2 << 20},
		{Name: "sodium.jar", ModTitle: "Sodium · 0.6.0", ModIDs: "renderer_mod", Enabled: true, Size: 1 << 20, Modified: time.Date(2026, 11, 18, 10, 30, 0, 0, time.Local)},
		{Name: "old.jar", Enabled: false},
	}}
	a.inspect(i)
	tt.Frame()
	if _, ok := tt.Find("加载器"); !ok {
		t.Fatal("loader settings are not available")
	}
	if err := tt.Click("资源"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	capture(t, tt, "fabric-mods")
	zone, ok := tt.Find("本地资源区")
	if !ok {
		t.Fatal("missing resource drop target")
	}
	x, y := zone.X+zone.W/2, zone.Y+zone.H/2
	a.pickerBusy = true
	tt.Frame()
	if tt.DropFiles(x, y, "demo.jar") {
		t.Fatal("accepted a drop during another file selection")
	}
	a.pickerBusy = false
	tt.Frame()
	if !tt.DropFiles(x, y, "demo.jar") || a.state.Selected != 1 || a.content.id != i.ID {
		t.Fatal("resource drop did not stay on the viewed instance")
	}
	tt.SetSize(820, 530)
	tt.SetDark(true)
	tt.Advance(500 * time.Millisecond)
	tt.Frame()
	for _, want := range []int{1, 0} {
		if err := tt.Click("选择资源 old.jar"); err != nil || len(a.content.selection(a.content.files)) != want {
			t.Fatalf("toggle disabled resource selection: want %d, error %v", want, err)
		}
	}
	if err := tt.Click("全选当前列表"); err != nil {
		t.Fatal(err)
	}
	if len(a.content.selection(a.content.files)) != 3 {
		t.Fatal("select all missed local resources")
	}
	tt.Advance(250 * time.Millisecond)
	tt.Frame()
	capture(t, tt, "resources-selected-compact")
	for _, order := range []struct{ label, first string }{{"文件较大", "demo.jar"}, {"最近修改", "sodium.jar"}} {
		if err := tt.Click("本地资源排序"); err != nil {
			t.Fatal(err)
		}
		if err := tt.Click(order.label); err != nil {
			t.Fatal(err)
		}
		if files := a.content.visibleFiles(); files[0].Name != order.first || len(a.content.selection(files)) != 3 {
			t.Fatal("sorting lost the selection or put the wrong file first")
		}
	}
	tt.Advance(250 * time.Millisecond)
	tt.Frame()
	capture(t, tt, "resources-sorted-compact")
	if err := tt.Click("搜索本地资源"); err != nil {
		t.Fatal(err)
	}
	tt.Type("RENDERER_MOD")
	if len(a.content.visibleFiles()) != 1 || len(a.content.selection(a.content.files)) != 0 {
		t.Fatal("filter did not narrow files and clear hidden selections")
	}
	if err := tt.Click("复制资源列表"); err != nil || tt.Clipboard() != "[启用] sodium.jar\n" {
		t.Fatalf("filtered list copy: %v", err)
	}
	a.content.query = ""
	tt.Frame()
	if err := tt.Click("已停用"); err != nil {
		t.Fatal(err)
	}
	if files := a.content.visibleFiles(); len(files) != 1 || files[0].Name != "old.jar" || a.state.Selected != 1 {
		t.Fatal("disabled filter or launch target changed")
	}
	capture(t, tt, "resources-disabled-compact")
	if err := tt.Click("添加模组…"); err != nil {
		t.Fatal(err)
	}
	p := a.browserFor(launcher.Mods)
	if a.page != pageDownloads || a.downloadPage != downloadMods || p.target.ID != i.ID || a.state.Selected != 1 {
		t.Fatal("mod download did not capture the inspected Fabric instance")
	}
	if err := a.state.AddServer("测试服务器", "localhost:25565"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("联机"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("启动并加入"); !ok || a.page != pageNetwork {
		t.Fatal("the saved Java server has no quick join entry")
	}
	tt.SetSize(820, 530)
	tt.Frame()
	for _, label := range []string{"选择联机实例", "建筑测试 · 1.21.4 · Fabric 0.16.14"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if target, ok := a.serverInstance(); !ok || target.ID != 2 || a.state.Selected != 1 {
		t.Fatal("server selection changed the home launch target or captured the wrong instance")
	}
	if err := tt.Click("复制服务器地址"); err != nil || tt.Clipboard() != "localhost:25565" {
		t.Fatalf("server address copy failed: %v", err)
	}
	capture(t, tt, "saved-servers")
	status := &serverCheck{updated: time.Date(2026, 11, 18, 8, 0, 0, 0, time.Local), description: "欢迎来到社团生存服\n本周六开放建筑展览。", summary: "Paper 1.21.4 · 12/80 人 · 36 ms"}
	a.serverStatus = map[string]*serverCheck{"localhost:25565": status}
	tt.Frame()
	capture(t, tt, "server-status-success")
	tt.SetSize(1140, 720)
	tt.SetDark(false)
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	capture(t, tt, "server-status-light")
	tt.SetSize(820, 530)
	tt.SetDark(true)
	tt.Advance(300 * time.Millisecond)
	status.err = "连接超时，请检查服务器地址或稍后重试。"
	tt.Frame()
	capture(t, tt, "server-query-failed")
	a.state.Remove(2)
	tt.Frame()
	if _, ok := a.serverInstance(); ok || !tt.HasText("原实例已移除，请重新选择") {
		t.Fatal("removing a pinned server instance silently switched the launch target")
	}
	capture(t, tt, "server-target-removed")
	for _, label := range []string{"选择联机实例", "跟随主页 · 生存世界 · 1.21.1 · 原版"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if target, ok := a.serverInstance(); !ok || target.ID != a.state.Selected || a.serverInstanceID != 0 {
		t.Fatal("could not follow the home target after removing a pinned instance")
	}
	a.inspectContent(a.state.Instances[0], launcher.ResourcePacks)
	tt.Frame()
	capture(t, tt, "resource-drop-empty-compact")
	if !tt.HasText(".zip 文件拖到此处") {
		t.Fatal("missing file type hint for an empty resource pack directory")
	}
}

func TestInstanceLogSelectionKeepsTargetAndResetsPreview(t *testing.T) {
	a, tt := testDesktop(t)
	i := a.state.Instances[1]
	a.inspect(i)
	a.detailPage = detailFiles
	tt.SetSize(820, 530)
	tt.Frame()
	capture(t, tt, "instance-files-compact")
	if err := tt.Click("日志与崩溃报告…"); err != nil {
		t.Fatal(err)
	}
	if a.logLibrary == nil || a.logLibrary.instance.ID != i.ID || a.state.Selected != 1 {
		t.Fatal("log browser did not capture the viewed instance")
	}
	capture(t, tt, "instance-logs-empty")
	a.logLibrary.err = "日志目录不可读"
	tt.Frame()
	if !tt.HasText(i.Name+" · "+i.GameDir) || !tt.HasText("检查实例目录和读取权限后，刷新日志列表重试。") {
		t.Fatal("directory failure lost the instance location or retry guidance")
	}
	capture(t, tt, "instance-log-directory-error")
	a.logLibrary.err = ""
	modified := time.Date(2026, 11, 18, 10, 30, 0, 0, time.Local)
	latest := filepath.Join(i.GameDir, "logs", "latest.log")
	crash := filepath.Join(i.GameDir, "crash-reports", "crash-test-client.txt")
	a.logLibrary.entries = []launcherlog.Entry{{Path: latest, Modified: modified}, {Path: crash, Modified: modified}}
	a.selectLog(latest)
	tt.Frame()
	if !tt.HasText("显示 0 行") || !tt.HasText("日志暂时为空。") {
		t.Fatal("empty file hint or line count is missing")
	}
	capture(t, tt, "instance-empty-log")
	a.logText, a.logQuery, a.logFilter = "old preview", "old", 1
	tt.Frame()
	if err := tt.Click("选择实例日志"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	tt.Frame()
	capture(t, tt, "instance-log-picker")
	if err := tt.Click("11-18 10:30 · 崩溃报告 · crash-test-client.txt"); err != nil {
		t.Fatal(err)
	}
	if a.logPath != crash || a.logText != "" || a.logQuery != "" || a.logFilter != 0 || a.state.Selected != 1 {
		t.Fatal("switching reports retained a stale preview or changed launch target")
	}
	a.logText = "---- Minecraft Crash Report ----\nDescription: Initializing game\n\njava.lang.RuntimeException: failed to load mod\n\tat demo.Client.start(Client.java:42)\n"
	tt.SetDark(true)
	tt.Frame()
	tt.Advance(500 * time.Millisecond)
	tt.Frame()
	capture(t, tt, "instance-crash-compact")
	a.logText = ""
	a.logError = "open " + strings.Repeat("模组整合包的实例目录/", 42) + "crash-reports/crash-test.txt: access denied"
	a.logExportErr = "目标文件夹没有写入权限，请选择其他保存位置。"
	a.logLibrary.err = "无法读取部分日志目录，请检查实例目录权限后刷新。"
	tt.Frame()
	capture(t, tt, "instance-log-errors-compact")
	tt.SetDark(false)
	tt.Advance(time.Second)
	tt.Frame()
	capture(t, tt, "instance-log-errors-compact-light")
	if err := tt.Click("复制日志错误详情"); err != nil || tt.Clipboard() != a.logError {
		t.Fatalf("copy full log error: %v", err)
	}
	if _, ok := tt.Find("无法读取日志"); !ok {
		t.Fatal("preview error is not visible")
	}
	if _, ok := tt.Find("无法导出日志"); ok {
		t.Fatal("secondary errors crowd the preview recovery action")
	}
	if err := tt.Click("关闭"); err != nil || a.logOpen {
		t.Fatalf("close action is not reachable with multiple errors: %v", err)
	}
	a.logDiagnosis = launcherlog.Diagnosis{Cause: "heap", Evidence: "OutOfMemoryError: Java heap space"}
	a.showInstanceLogs(a.state.Instances[0])
	tt.Frame()
	if _, ok := tt.Find("查看内存设置"); ok || a.logDiagnosis.Cause != "" {
		t.Fatal("an empty instance inherited the previous log diagnosis")
	}
	tt.Key(0, ui.KeyEscape)
	if a.logOpen {
		t.Fatal("Escape did not close instance logs")
	}
	a.showLog(filepath.Join(i.GameDir, "launcher-logs", "game-test.log"))
	if a.logLibrary != nil {
		t.Fatal("task log retained an unrelated instance catalog")
	}
}

func TestTaskHistoryReadErrorReplacesEmptyState(t *testing.T) {
	a, tt := testDesktop(t)
	a.historyReadErr = "unexpected end of JSON input"
	a.notice, a.noticeError, a.noticeHistory = a.historyReadErr, true, true
	tt.SetSize(820, 530)
	tt.Frame()
	if err := tt.Click("查看任务记录"); err != nil {
		t.Fatal(err)
	}
	for _, dark := range []bool{false, true} {
		tt.SetDark(dark)
		tt.Advance(time.Second)
		tt.Frame()
		if tt.HasText("暂无任务") || !tt.HasText("重新读取任务记录") {
			t.Fatal("unreadable history should show a recovery action")
		}
		name := "task-history-error-light"
		if dark {
			name = "task-history-error-dark"
		}
		capture(t, tt, name)
	}
	if err := tt.Click("复制任务记录读取错误"); err != nil || tt.Clipboard() != a.historyReadErr {
		t.Fatalf("copy task history error: %v", err)
	}
}

func capture(t *testing.T, tt *ui.Tester, name string) {
	dir := os.Getenv("LXMCL_UI_CAPTURE")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

func TestGameServerJoinFollowsTheVisibleInstance(t *testing.T) {
	a, tt := testDesktop(t)
	a.serverInstanceID = a.state.Instances[1].ID
	a.serverSource = 1
	target := a.state.Instances[1]
	rows := []launcher.Server{{Name: "建筑", Address: "build.example.invalid:25570"}, {Name: "空地址"}}
	if err := tt.Click("联机"); err != nil {
		t.Fatal(err)
	}
	a.gameServers = &gameServerView{instance: target, rows: rows}
	tt.Frame()
	join := "启动并加入 建筑 build.example.invalid:25570"
	if b, ok := tt.Access(join); !ok || b.Disabled {
		t.Fatal("missing join action for the selected instance")
	}
	if b, ok := tt.Access("启动并加入 空地址 未填写地址"); !ok || !b.Disabled {
		t.Fatal("empty address enabled")
	}
	a.serverInstanceID = a.state.Instances[0].ID
	tt.Frame()
	if _, ok := tt.Access(join); ok {
		t.Fatal("previous instance still offers its join action")
	}
}

func TestServerDialogsKeepTemporaryAndSavedTargetsSeparate(t *testing.T) {
	a, tt := testDesktop(t)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	click("联机")
	capture(t, tt, "server-empty-light")
	click("临时直连…")
	tt.Type("not a valid address")
	click("启动并连接")
	if a.serverForm == nil || a.serverForm.err == "" || len(a.state.Servers) != 0 {
		t.Fatal("invalid direct address was accepted or saved")
	}
	tt.Key(0, ui.KeyF1)
	if a.page != pageNetwork {
		t.Fatal("global navigation escaped the direct connection dialog")
	}
	click("选择联机实例")
	click("生存世界 · 1.21.1 · 原版")
	a.state.Selected = 2
	tt.Frame()
	if target, ok := a.serverInstance(); !ok || target.ID != 1 {
		t.Fatal("explicitly choosing the home instance did not pin it")
	}
	click("选择联机实例")
	click("跟随主页 · 建筑测试 · 1.21.4 · 原版")
	a.state.Selected = 1
	tt.Frame()
	if target, ok := a.serverInstance(); !ok || target.ID != 1 || a.serverInstanceID != 0 {
		t.Fatal("server selection did not resume following home")
	}
	click("选择联机实例")
	click("建筑测试 · 1.21.4 · 原版")
	if a.serverInstanceID != 2 || a.state.Selected != 1 {
		t.Fatal("temporary connection changed the home launch target")
	}
	tt.Key(0, ui.KeyEscape)
	if a.serverForm != nil {
		t.Fatal("Escape did not close direct connection")
	}
	auto := launcher.Server{Name: "old.example:25565", Address: "old.example:25565"}
	a.openServerForm(serverEdit, auto)
	if a.serverForm.name != "" || a.serverForm.original != auto {
		t.Fatal("editing an address-derived favorite turned its old address into a custom name")
	}
	a.serverForm = nil
	if err := a.state.AddServer("社团生存服", "play.example.org"); err != nil {
		t.Fatal(err)
	}
	tt.SetSize(820, 530)
	tt.SetDark(true)
	tt.Frame()
	click("编辑服务器 社团生存服")
	if a.serverForm.original != a.state.Servers[0] || a.serverForm.address != "play.example.org:25565" || a.serverForm.name != "社团生存服" {
		t.Fatal("edit did not capture the selected server")
	}
	capture(t, tt, "server-edit-dark")
	click("取消")
	click("移除服务器 社团生存服")
	click("取消")
	if a.serverForm != nil || len(a.state.Servers) != 1 {
		t.Fatal("canceling removal changed favorites")
	}
	click("添加服务器…")
	tt.Type("PLAY.EXAMPLE.ORG")
	click("添加到收藏")
	if a.serverForm == nil || a.serverForm.err == "" || len(a.state.Servers) != 1 {
		t.Fatal("duplicate favorite was accepted")
	}
	click("取消")
	click("临时直连…")
	tt.Type("[2001:db8::2]:25566")
	capture(t, tt, "server-direct-dark")
}

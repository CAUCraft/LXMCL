package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/CAUCraft/LXMCL/internal/tasks"
)

func TestQueueRestartRetainsTargetsAndPrunesFinishedWork(t *testing.T) {
	root := t.TempDir()
	res := resourceTask{Kind: string(launcher.Mods), Project: "project", Version: "build", Title: "Sodium"}
	i := launcher.Instance{ID: 17, Revision: 3, Version: "1.21.1", GameDir: filepath.Join(root, "game"), Name: "World"}
	v := launcher.PackVersion{Number: "0.6"}
	curse := res
	curse.Source, curse.Project, curse.Version = launcher.CurseForgeMods, "248787", "5864741"
	entries := []queuedResource{{id: "done", resource: res, folder: root, version: v}, {id: "interrupted", resource: curse, target: i, version: v}, {id: "waiting", resource: res, folder: root, version: v}}
	if err := writeDownloadQueue(root, queueEntries(entries)); err != nil {
		t.Fatal(err)
	}
	history := []taskRecord{{ID: "done", Status: taskCompleted}, {ID: "interrupted", Status: tasks.WaitingForKey}, {ID: "waiting", Status: taskCanceled}}
	for range 2 {
		items, err := readDownloadQueue(root, history)
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 2 || items[0].saved() != entries[1].saved() || items[1].saved() != entries[2].saved() {
			t.Fatalf("restored entries: %+v", items)
		}
		history = nil
	}
	path := filepath.Join(root, "download-queue.json")
	if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readDownloadQueue(root, nil); err == nil {
		t.Fatal("invalid queue accepted")
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "{" {
		t.Fatal("invalid queue overwritten")
	}
	first, _ := json.Marshal(entries[1].saved())
	last, _ := json.Marshal(entries[2].saved())
	torn := []byte("[" + string(first) + ",42," + string(last) + ",")
	if err := os.WriteFile(path, torn, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readDownloadQueue(root, nil); !errors.Is(err, errQueueFormat) {
		t.Fatalf("invalid queue was not flagged: %v", err)
	}
	restored, backup, err := repairDownloadQueue(root, nil)
	if err != nil || len(restored) != 2 || restored[0].id != "interrupted" || restored[1].id != "waiting" {
		t.Fatalf("repaired entries: %v %v", restored, err)
	}
	if b, err := os.ReadFile(filepath.Join(root, backup)); err != nil || !bytes.Equal(b, torn) {
		t.Fatalf("original bytes lost: %v", err)
	}
	if saved, err := readDownloadQueue(root, nil); err != nil || len(saved) != 2 {
		t.Fatalf("reloaded repaired queue: %v %v", saved, err)
	}
}

func TestQueueWaitsForCurseForgeKey(t *testing.T) {
	a, tt := testDesktop(t)
	tt.SetSize(820, 530)
	target := a.state.Instances[1]
	target.Loader, target.LoaderVersion = launcher.LoaderFabric, "0.16.14"
	a.state.Instances[1] = target
	item := queuedResource{id: "waiting", target: target, version: launcher.PackVersion{Number: "3.0"},
		resource: resourceTask{Source: launcher.CurseForgeMods, Kind: string(launcher.Mods), Title: "AppleSkin", Project: "248787", Version: "5864741"}}
	waiting := taskRecord{ID: item.id, InstanceID: target.ID, Revision: target.Revision, Version: target.Version, Name: target.Name,
		Action: "安装模组", Status: tasks.WaitingForKey, Resource: &item.resource, Detail: "请更新 CurseForge API Key。", At: time.Now()}
	a.history = []taskRecord{waiting, {ID: "previous", Name: "先前的任务", Status: taskFailed, Action: taskVerify, At: waiting.At.Add(-time.Hour)}}
	root := t.TempDir()
	if err := writeDownloadQueue(root, queueEntries([]queuedResource{item})); err != nil {
		t.Fatal(err)
	}
	var err error
	a.queue.pending, err = readDownloadQueue(root, a.history)
	if err != nil {
		t.Fatal(err)
	}
	a.inspect(a.state.Instances[0])
	a.queue.running = true
	a.ctx = t.Context()
	a.nextResource()
	a.ctx = nil
	if a.queue.running || a.queue.activeID != "" || len(a.queue.pending) != 1 || a.busy {
		t.Fatal("missing key consumed or started the waiting download")
	}
	a.taskFilter, a.browsedTask = 1, "previous"
	a.notice, a.noticeError, a.noticeTaskKey = waiting.Detail, true, waiting.ID
	tt.Frame()
	if err := tt.Click("查看任务"); err != nil || !tt.HasText("CurseForge · AppleSkin") {
		t.Fatalf("notice did not reveal the waiting record in filtered history: %v", err)
	}
	if err := tt.Click("失败 / 取消"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("查看任务"); err != nil || a.browsedTask != waiting.ID || !tt.HasText("CurseForge · AppleSkin") {
		t.Fatalf("could not reopen the filtered record from the task center: %v", err)
	}
	for _, theme := range []string{"light", "dark"} {
		tt.SetDark(theme == "dark")
		tt.Frame()
		tt.Advance(300 * time.Millisecond)
		tt.Frame()
		capture(t, tt, "queue-waiting-for-key-"+theme)
	}
	if err := tt.Click("设置 CurseForge API Key…"); err != nil {
		t.Fatal(err)
	}
	a.modpacks.keyDraft = "fixture"
	if err := tt.Click("应用"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("开始队列"); !ok || a.needsResourceKey(item.resource) || a.queue.pending[0].saved() != item.saved() || a.state.Selected != 1 || a.inspected != 1 {
		t.Fatal("applying a key lost the waiting download")
	}
}

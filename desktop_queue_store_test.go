package main

import (
	"github.com/CAUCraft/LXMCL/internal/launcher"
	"os"
	"path/filepath"
	"testing"
)

func TestQueueRestartRetainsTargetsAndPrunesFinishedWork(t *testing.T) {
	root := t.TempDir()
	res := resourceTask{Kind: string(launcher.Mods), Project: "project", Version: "build", Title: "Sodium"}
	i := launcher.Instance{ID: 17, Revision: 3, Version: "1.21.1", GameDir: filepath.Join(root, "game"), Name: "World"}
	v := launcher.PackVersion{Number: "0.6"}
	entries := []queuedResource{{id: "done", resource: res, folder: root, version: v}, {id: "interrupted", resource: res, target: i, version: v}, {id: "waiting", resource: res, folder: root, version: v}}
	if err := writeDownloadQueue(root, queueEntries(entries)); err != nil {
		t.Fatal(err)
	}
	history := []taskRecord{{ID: "done", Status: taskCompleted}, {ID: "interrupted", Status: taskCanceled}}
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
}

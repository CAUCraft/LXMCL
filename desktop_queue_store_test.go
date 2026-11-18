package main

import (
	"bytes"
	"encoding/json"
	"errors"
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
	curse := res
	curse.Source, curse.Project, curse.Version = launcher.CurseForgeMods, "248787", "5864741"
	entries := []queuedResource{{id: "done", resource: res, folder: root, version: v}, {id: "interrupted", resource: curse, target: i, version: v}, {id: "waiting", resource: res, folder: root, version: v}}
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

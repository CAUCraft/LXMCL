package tasks

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestJournalKeepsUnreadableHistoryUntilItCanBeLoaded(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "tasks.json")
	corrupt := []byte(`[{"id":"old-task","status":`)
	if err := os.WriteFile(path, corrupt, 0600); err != nil {
		t.Fatal(err)
	}
	var journal Journal
	if _, err := journal.Load(root); err == nil {
		t.Fatal("accepted incomplete history")
	}
	newTask := (Record{Name: "new-task", Status: Completed}).Submitted()
	if _, err := journal.Record(root, newTask); err == nil {
		t.Fatal("overwrote history after a failed load")
	}
	if data, err := os.ReadFile(path); err != nil || !bytes.Equal(data, corrupt) {
		t.Fatal("failed update modified the original file")
	}
	oldTask := (Record{Name: "old-task", Status: Completed}).Submitted()
	if err := Save(root, []Record{oldTask}); err != nil {
		t.Fatal(err)
	}
	records, err := journal.Record(root, newTask)
	if err != nil || len(records) != 2 || records[1].ID != oldTask.ID {
		t.Fatalf("did not recover after repaired history: %v", err)
	}
}

func TestSnapshotDoesNotWaitForTheHistoryWriter(t *testing.T) {
	var journal Journal
	resource := &Resource{Title: "submitted"}
	row := (Record{Resource: resource}).Submitted()
	if _, err := journal.Record(t.TempDir(), row); err != nil {
		t.Fatal(err)
	}
	// The writer holds this lock across file creation and Sync. Holding it here
	// models a blocked filesystem without depending on the host's disk speed.
	journal.mu.Lock()
	defer journal.mu.Unlock()
	done := make(chan []Record, 1)
	go func() { done <- journal.Snapshot() }()
	select {
	case records := <-done:
		records[0].Resource.Title = "view edit"
	case <-time.After(time.Second):
		t.Fatal("reading history waited for disk synchronization")
	}
	row.Resource.Title = "caller edit"
	if journal.Snapshot()[0].Resource.Title != "submitted" {
		t.Fatal("caller changed an immutable history snapshot")
	}
}

func TestConcurrentHistoryUpdatesKeepEveryTask(t *testing.T) {
	var journal Journal
	root := t.TempDir()
	var writers sync.WaitGroup
	for range 8 {
		writers.Go(func() {
			row := (Record{Resource: &Resource{Title: "submitted"}}).Submitted()
			if _, err := journal.Record(root, row); err != nil {
				t.Error(err)
			}
			row.Resource.Title = "caller edit"
		})
	}
	for range 32 {
		_ = journal.Snapshot()
	}
	writers.Wait()
	records, err := Load(root)
	if err != nil || len(records) != 8 {
		t.Fatalf("lost concurrent results: %d records, %v", len(records), err)
	}
	for _, row := range journal.Snapshot() {
		if row.Resource.Title != "submitted" {
			t.Fatal("history retained caller-owned data")
		}
	}
}

func TestHistoryRejectsTrailingDataAndOversizedFiles(t *testing.T) {
	for _, data := range []string{`[]garbage`, `[][]`, "[]" + strings.Repeat(" ", maxHistorySize)} {
		root := t.TempDir()
		path := filepath.Join(root, "tasks.json")
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		var journal Journal
		if _, err := journal.Record(root, (Record{}).Submitted()); err == nil {
			t.Fatal("accepted invalid history")
		}
		if got, err := os.ReadFile(path); err != nil || string(got) != data {
			t.Fatal("invalid history was overwritten")
		}
	}
}

func TestOversizedResultPreservesSavedHistory(t *testing.T) {
	root := t.TempDir()
	var journal Journal
	row := (Record{Status: Started}).Submitted()
	if _, err := journal.Record(root, row); err != nil {
		t.Fatal(err)
	}
	row.Status, row.Detail = Failed, strings.Repeat("x", maxHistorySize)
	if _, err := journal.Record(root, row); err == nil {
		t.Fatal("saved a file that cannot be read on restart")
	}
	records, err := Load(root)
	if err != nil || len(records) != 1 || records[0].Status != Started {
		t.Fatalf("oversized result replaced saved history: %v", err)
	}
	if records := journal.Snapshot(); len(records) != 1 || records[0].Status != Failed {
		t.Fatal("persistence failure erased the actual task result")
	}
}

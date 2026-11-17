package main

import (
	"testing"
	"time"
)

func TestTaskHistoryPersistsExitAndBounds(t *testing.T) {
	root := t.TempDir()
	var records []taskRecord
	start := time.Now()
	for n := range historyLimit + 3 {
		records = updateTaskHistory(records, taskRecord{At: start.Add(time.Duration(n) * time.Minute), Status: taskCompleted})
	}
	run := taskRecord{At: start.Add(time.Hour), Name: "测试实例", Version: "1.21.1", Status: taskStarted, PID: 123}
	records = updateTaskHistory(records, run)
	if err := saveTaskHistory(root, records); err != nil {
		t.Fatal(err)
	}
	records, err := loadTaskHistory(root)
	if err != nil {
		t.Fatal(err)
	}
	run.Status = taskExited
	if err := saveTaskHistory(root, updateTaskHistory(records, run)); err != nil {
		t.Fatal(err)
	}
	records, err = loadTaskHistory(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != historyLimit || records[0].Status != taskExited || records[0].PID != run.PID {
		t.Fatalf("unexpected history: %+v", records)
	}
	for _, item := range records[1:] {
		if item.At.Equal(run.At) {
			t.Fatal("exit should update the original launch record")
		}
	}
}

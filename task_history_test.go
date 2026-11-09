package main

import (
	"os"
	"path/filepath"
	"strings"
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

func TestGameLogPreviewReadsBoundedTail(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "launcher-logs")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "game-test.log")
	prefix := "this line must not be shown"
	b := []byte(prefix + "\n" + strings.Repeat("一次游戏事件\n", logTailLimit) + "客户端退出\n")
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	log, err := readGameLog(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(log, prefix) || !strings.HasSuffix(log, "客户端退出\n") || len(log) > logTailLimit+128 {
		t.Fatalf("unexpected log preview of %d bytes", len(log))
	}
	if _, err := readGameLog(filepath.Join(dir, "other.exe")); err == nil {
		t.Fatal("accepted a non-log path")
	}
}

package tasks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestJournalUpdatesOriginalTaskWithoutLosingNewerRecords(t *testing.T) {
	var journal Journal
	root := t.TempDir()
	started := (Record{At: time.Now(), Status: Started, Log: "game.log", PID: 7}).Submitted()
	another := (Record{At: started.At, Status: Completed}).Submitted()
	if _, err := journal.Record(root, started); err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Record(root, another); err != nil {
		t.Fatal(err)
	}
	started.Status = Exited
	if _, err := journal.Record(root, started); err != nil {
		t.Fatal(err)
	}
	records, err := Load(root)
	if err != nil || len(records) != 2 || records[0].ID != started.ID || records[1].ID != another.ID || records[0].PID != 7 {
		t.Fatalf("lost task or process result: %v %v", records, err)
	}
}
func TestLegacyHistoryKeepsIdentity(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "tasks.json"), []byte(`[{"at":"2026-01-01T00:00:00Z","status":"已失败","action":"启动游戏","instance_id":4,"instance_revision":2,"resource":{"kind":"mods","project":"modA","version":"releaseA","title":"Mod A"}},{"id":"future","status":"已失败","resource":{"kind":"future-kind","project":"other","version":"v1"}}]`), 0600); err != nil {
		t.Fatal(err)
	}
	records, err := Load(root)
	if err != nil || len(records) != 2 || records[0].Status != Failed || records[0].Revision != 2 ||
		records[0].Resource == nil || records[0].Resource.Kind != "mods" ||
		records[1].Resource == nil || records[1].Resource.Kind != "future-kind" {
		t.Fatalf("legacy: %v %v", records, err)
	}
	if err := Save(root, records); err != nil {
		t.Fatal(err)
	}
	if saved, err := Load(root); err != nil || !reflect.DeepEqual(saved, records) {
		t.Fatalf("history round trip: %v %v", saved, err)
	}
	updated := records[0]
	updated.Status = Completed
	if got := Update(records, updated); len(got) != 2 || got[0].Key() != records[0].Key() {
		t.Fatal("duplicated legacy task")
	}
}
func TestRunStopsProgressAfterCompletionAndHonorsCancellation(t *testing.T) {
	var late func(Progress)
	count := 0
	if err := Run(t.Context(), func(ctx context.Context, report func(Progress)) error {
		late = report
		report(Progress{Done: 1})
		return nil
	}, func(Progress) { count++ }); err != nil {
		t.Fatal(err)
	}
	late(Progress{Done: 2})
	if count != 1 {
		t.Fatal("late progress delivered")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := Run(ctx, func(context.Context, func(Progress)) error { t.Fatal("canceled work started"); return nil }, nil)
	if !errors.Is(err, context.Canceled) || Outcome(err) != Canceled || Outcome(context.DeadlineExceeded) != Failed {
		t.Fatalf("outcome: %v", err)
	}
}

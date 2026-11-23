package playtime

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecordObservedSessions(t *testing.T) {
	root := t.TempDir()
	start := time.Now()
	for _, update := range []struct {
		id       int
		start    time.Time
		duration time.Duration
	}{
		{1, start, 0}, {1, start, 3 * time.Second}, {1, start, 3 * time.Second},
		{2, start.Add(time.Hour), 500 * time.Millisecond}, {1, start.Add(2 * time.Hour), 2 * time.Second},
	} {
		if _, err := Record(root, update.id, update.start, update.duration); err != nil {
			t.Fatal(err)
		}
	}
	got, err := Load(root)
	if err != nil || got[1].TotalMillis() != 5000 || !got[1].LastPlayed.Equal(start.Add(2*time.Hour)) || got[2].TotalMillis() != 500 {
		t.Fatal(got, err)
	}
	path := filepath.Join(root, "playtime.json")
	if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Record(root, 1, start, time.Second); err == nil {
		t.Fatal("unreadable history replaced")
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "{" {
		t.Fatal(string(b), err)
	}
}

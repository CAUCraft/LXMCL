// Package playtime stores locally observed game sessions separately from launch configuration.
package playtime

import (
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/CAUCraft/LXMCL/internal/storage"
)

type Entry struct {
	LastPlayed     time.Time `json:"last_played"`
	PreviousMillis int64     `json:"previous_ms,omitempty"`
	SessionMillis  int64     `json:"session_ms,omitempty"`
}

func (e Entry) TotalMillis() int64 { return e.PreviousMillis + e.SessionMillis }

func Load(root string) (map[int]Entry, error) {
	var entries map[int]Entry
	_, err := storage.ReadJSON(filepath.Join(root, "playtime.json"), &entries)
	return entries, err
}

// Record receives the full observed duration for one session, first at startup
// and again at exit or launcher shutdown. The launch worker serializes calls.
func Record(root string, id int, start time.Time, elapsed time.Duration) (map[int]Entry, error) {
	entries, err := Load(root)
	if err != nil {
		return nil, err
	}
	if entries == nil {
		entries = make(map[int]Entry)
	}
	e := entries[id]
	if !e.LastPlayed.Equal(start) {
		e = Entry{LastPlayed: start.UTC(), PreviousMillis: e.TotalMillis()}
	}
	e.SessionMillis = max(e.SessionMillis, elapsed.Milliseconds())
	entries[id] = e
	b, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return nil, err
	}
	return entries, storage.WriteFile(filepath.Join(root, "playtime.json"), b)
}

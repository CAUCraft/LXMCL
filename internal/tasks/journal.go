package tasks

import (
	"sync"
	"sync/atomic"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

// Journal serializes history updates so an old task finishing cannot replace
// newer records. Use one Journal for each application data directory.
type Journal struct {
	mu       sync.Mutex
	root     string
	loaded   bool
	snapshot atomic.Pointer[historySnapshot]
}

// Published records are immutable. Readers never wait for a writer's disk I/O.
type historySnapshot struct{ records []Record }

func (j *Journal) load(root string) error {
	if j.loaded {
		if root != j.root {
			return i18n.Error("tasks.journal_root_changed")
		}
		return nil
	}
	records, err := Load(root)
	if err != nil {
		return err
	}
	j.root, j.loaded = root, true
	j.publish(records)
	return nil
}

func (j *Journal) Load(root string) ([]Record, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	err := j.load(root)
	return j.Snapshot(), err
}

func (j *Journal) Record(root string, record Record) ([]Record, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := j.load(root); err != nil {
		return j.Snapshot(), err
	}
	records := clone(Update(j.current(), record))
	j.publish(records)
	return clone(records), Save(root, records)
}

// Snapshot returns an independent in-memory view, including results whose disk
// write failed. Persistence errors are reported separately by Record.
func (j *Journal) Snapshot() []Record {
	return clone(j.current())
}

func (j *Journal) current() []Record {
	if snapshot := j.snapshot.Load(); snapshot != nil {
		return snapshot.records
	}
	return nil
}

func (j *Journal) publish(records []Record) {
	j.snapshot.Store(&historySnapshot{records: records})
}

func clone(records []Record) []Record {
	result := append([]Record(nil), records...)
	for n := range result {
		if result[n].Resource != nil {
			r := *result[n].Resource
			result[n].Resource = &r
		}
	}
	return result
}

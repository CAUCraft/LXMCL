package tasks

import (
	"sync"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

// Journal serializes history updates so an old task finishing cannot replace
// newer records. Use one Journal for each application data directory.
type Journal struct {
	mu      sync.Mutex
	root    string
	loaded  bool
	records []Record
}

func (j *Journal) load(root string) error {
	if j.loaded {
		if root != j.root {
			return i18n.Error("tasks.journal_root_changed")
		}
		return nil
	}
	records, err := Load(root)
	j.root, j.loaded, j.records = root, true, records
	return err
}

func (j *Journal) Load(root string) ([]Record, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	err := j.load(root)
	return clone(j.records), err
}

func (j *Journal) Record(root string, record Record) ([]Record, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := j.load(root); err != nil {
		return clone(j.records), err
	}
	j.records = Update(j.records, record)
	return clone(j.records), Save(root, j.records)
}

func (j *Journal) Snapshot() []Record {
	j.mu.Lock()
	defer j.mu.Unlock()
	return clone(j.records)
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

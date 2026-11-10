package tasks

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

const maxHistorySize = 2 << 20

func Load(root string) ([]Record, error) {
	f, err := os.Open(filepath.Join(root, "tasks.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxHistorySize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxHistorySize {
		return nil, i18n.Error("tasks.history_too_large")
	}
	var records []Record
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	return records[:min(len(records), Limit)], nil
}

func Update(records []Record, record Record) []Record {
	next := []Record{record}
	for _, item := range records {
		if item.Key() != record.Key() {
			next = append(next, item)
		}
		if len(next) == Limit {
			break
		}
	}
	return next
}

func Save(root string, records []Record) error {
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if len(data) > maxHistorySize {
		return i18n.Error("tasks.history_too_large")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	return storage.WriteFile(filepath.Join(root, "tasks.json"), data)
}

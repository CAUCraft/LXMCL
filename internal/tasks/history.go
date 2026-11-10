package tasks

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/i18n"
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
	f, err := os.CreateTemp(root, ".lxmcl-tasks-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if err := errors.Join(err, f.Close()); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(root, "tasks.json"))
}

package tasks

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

func Load(root string) ([]Record, error) {
	f, err := os.Open(filepath.Join(root, "tasks.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var records []Record
	if err := json.NewDecoder(io.LimitReader(f, 2<<20)).Decode(&records); err != nil {
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
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(root, ".lxmcl-tasks-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	err = enc.Encode(records)
	if err == nil {
		err = f.Sync()
	}
	if err := errors.Join(err, f.Close()); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(root, "tasks.json"))
}

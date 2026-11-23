package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

// The queue persists the submitted destination and provider IDs. Download URLs
// are resolved again from the pinned version when the launcher restarts.
type savedDownload struct {
	ID         string       `json:"id"`
	Resource   resourceTask `json:"resource"`
	Number     string       `json:"number,omitempty"`
	InstanceID int          `json:"instance_id,omitempty"`
	Revision   uint64       `json:"revision,omitempty"`
	Name       string       `json:"instance_name,omitempty"`
	Version    string       `json:"game_version,omitempty"`
	GameDir    string       `json:"game_directory,omitempty"`
	Folder     string       `json:"folder,omitempty"`
}

func (q queuedResource) saved() savedDownload {
	return savedDownload{ID: q.id, Resource: q.resource, Number: q.version.Number, InstanceID: q.target.ID, Revision: q.target.Revision, Name: q.target.Name, Version: q.target.Version, GameDir: q.target.GameDir, Folder: q.folder}
}

const maxQueueBytes = 8 << 20
const queueFilename = "download-queue.json"

var errQueueFormat = errors.New("队列格式无效")

func readDownloadQueue(root string, history []taskRecord) ([]queuedResource, error) {
	data, err := queueBytes(filepath.Join(root, queueFilename))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	pending, prune, err := decodeQueue(data, history)
	if err != nil {
		return nil, err
	}
	if prune {
		err = writeDownloadQueue(root, queueEntries(pending))
	}
	return pending, err
}

func queueBytes(path string) ([]byte, error) {
	st, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, errors.New("队列路径不是普通文件")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, maxQueueBytes+1))
	return data, errors.Join(err, f.Close())
}

func (e savedDownload) valid() bool {
	if e.ID == "" || e.Resource.Project == "" || e.Resource.Version == "" || !slices.Contains([]launcher.ContentKind{launcher.Mods, launcher.ResourcePacks, launcher.ShaderPacks}, launcher.ContentKind(e.Resource.Kind)) {
		return false
	}
	if e.InstanceID == 0 {
		return filepath.IsAbs(e.Folder) && e.GameDir == ""
	}
	return e.InstanceID > 0 && filepath.IsAbs(e.GameDir) && e.Folder == "" && e.Version != ""
}

// Return complete valid entries, including those preceding a truncated array.
// Only explicit repair may use these entries when format validation fails.
func decodeQueue(data []byte, history []taskRecord) (pending []queuedResource, prune bool, formatErr error) {
	if len(data) <= maxQueueBytes && bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil, false, nil
	}
	if len(data) > maxQueueBytes {
		formatErr = fmt.Errorf("%w：超过 8 MiB", errQueueFormat)
		data = data[:maxQueueBytes]
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	if token, err := dec.Token(); err != nil || token != json.Delim('[') {
		return nil, false, errQueueFormat
	}
	finished, seen := map[string]bool{}, map[string]bool{}
	for _, r := range history {
		if r.Status == taskCompleted || r.Status == taskFailed {
			finished[r.ID] = true
		}
	}
	for index := 1; dec.More(); index++ {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return pending, prune, fmt.Errorf("%w：第 %d 项内容不完整或有误", errQueueFormat, index)
		}
		var e savedDownload
		if json.Unmarshal(raw, &e) != nil || !e.valid() || seen[e.ID] {
			formatErr = fmt.Errorf("%w：含无效或重复条目", errQueueFormat)
			continue
		}
		seen[e.ID] = true
		if finished[e.ID] {
			prune = true
			continue
		}
		target := launcher.Instance{ID: e.InstanceID, Revision: e.Revision, Name: e.Name, Version: e.Version, GameDir: e.GameDir}
		pending = append(pending, queuedResource{id: e.ID, resource: e.Resource, folder: e.Folder, version: launcher.PackVersion{Number: e.Number}, target: target})
	}
	if token, err := dec.Token(); err != nil || token != json.Delim(']') {
		return pending, prune, fmt.Errorf("%w：条目列表不完整", errQueueFormat)
	}
	if _, err := dec.Token(); err != io.EOF {
		return pending, prune, errQueueFormat
	}
	return pending, prune, formatErr
}

func writeDownloadQueue(root string, entries []savedDownload) error {
	data, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	if len(data) > maxQueueBytes {
		return fmt.Errorf("下载队列文件过大")
	}
	return storage.WriteFile(filepath.Join(root, queueFilename), data)
}

func queueEntries(items []queuedResource) []savedDownload {
	entries := make([]savedDownload, len(items))
	for n, item := range items {
		entries[n] = item.saved()
	}
	return entries
}

func (a *desktop) saveDownloadQueue() {
	q := &a.queue
	entries := queueEntries(q.pending)
	root := a.store.Root
	previous := q.lastWrite
	finished := make(chan struct{})
	q.lastWrite = finished
	a.writes.Go(func() {
		if previous != nil {
			<-previous
		}
		err := writeDownloadQueue(root, entries)
		close(finished)
		a.post(func() {
			if q.lastWrite != finished {
				return
			}
			q.lastWrite = nil
			q.saveError = err
			if err != nil {
				q.running = false
				a.message("保存下载队列失败", err)
				return
			}
			a.nextResource()
		})
	})
}

func repairDownloadQueue(root string, history []taskRecord) ([]queuedResource, string, error) {
	pending, err := readDownloadQueue(root, history)
	if !errors.Is(err, errQueueFormat) {
		return pending, "", err
	}
	dir, err := os.OpenRoot(root)
	if err != nil {
		return nil, "", err
	}
	defer dir.Close()
	st, err := dir.Lstat(queueFilename)
	if err != nil {
		return nil, "", err
	}
	if !st.Mode().IsRegular() {
		return nil, "", errors.New("队列路径不是普通文件")
	}
	backup := "download-queue-backup-" + rand.Text() + ".json"
	err = storage.PublishNew(context.Background(), dir, backup, 0600, func(dst *os.File) error {
		f, err := dir.Open(queueFilename)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.CopyN(dst, f, st.Size())
		return err
	})
	if err != nil {
		return nil, "", err
	}
	data, err := queueBytes(filepath.Join(root, backup))
	if err != nil {
		return nil, backup, err
	}
	pending, _, _ = decodeQueue(data, history)
	current, err := dir.Lstat(queueFilename)
	if err != nil {
		return nil, backup, err
	}
	if !os.SameFile(st, current) || st.Size() != current.Size() || !st.ModTime().Equal(current.ModTime()) {
		return nil, backup, errors.New("队列文件已更改，请重新读取")
	}
	err = writeDownloadQueue(root, queueEntries(pending))
	return pending, backup, err
}
func (a *desktop) reloadDownloadQueue(repair bool) {
	q := &a.queue
	if q.loading {
		return
	}
	q.loading = true
	history, root := slices.Clone(a.history), a.store.Root
	a.writes.Go(func() {
		var items []queuedResource
		var backup string
		var err error
		if repair {
			items, backup, err = repairDownloadQueue(root, history)
		} else {
			items, err = readDownloadQueue(root, history)
		}
		a.post(func() {
			q.loading = false
			q.loadError = err
			if err == nil {
				q.pending = items
			}
			if backup != "" {
				if err == nil {
					a.message(fmt.Sprintf("已恢复 %d 项 · 原文件备份为 %s", len(items), backup), nil)
				} else {
					a.message("原文件已备份为 "+backup, err)
				}
			}
		})
	})
}

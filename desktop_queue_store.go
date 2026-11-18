package main

import (
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

func readDownloadQueue(root string, history []taskRecord) ([]queuedResource, error) {
	f, err := os.Open(filepath.Join(root, "download-queue.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, maxQueueBytes+1))
	err = errors.Join(err, f.Close())
	if err != nil {
		return nil, err
	}
	if len(data) > maxQueueBytes {
		return nil, fmt.Errorf("下载队列文件过大")
	}
	var entries []savedDownload
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	finished := make(map[string]bool)
	for _, row := range history {
		if row.Status == taskCompleted || row.Status == taskFailed {
			finished[row.ID] = true
		}
	}
	seen := make(map[string]bool)
	var pending []queuedResource
	for _, e := range entries {
		if e.ID == "" || seen[e.ID] || e.Resource.Project == "" || e.Resource.Version == "" || !slices.Contains([]launcher.ContentKind{launcher.Mods, launcher.ResourcePacks, launcher.ShaderPacks}, launcher.ContentKind(e.Resource.Kind)) ||
			(e.InstanceID == 0 && !filepath.IsAbs(e.Folder)) || (e.InstanceID != 0 && (e.InstanceID < 0 || !filepath.IsAbs(e.GameDir) || e.Folder != "" || e.Version == "")) {
			return nil, fmt.Errorf("下载队列条目无效：%s", e.Resource.Title)
		}
		seen[e.ID] = true
		if !finished[e.ID] {
			pending = append(pending, queuedResource{id: e.ID, resource: e.Resource, folder: e.Folder, version: launcher.PackVersion{Number: e.Number}, target: launcher.Instance{ID: e.InstanceID, Revision: e.Revision, Name: e.Name, Version: e.Version, GameDir: e.GameDir}})
		}
	}
	if len(pending) != len(entries) {
		if err := writeDownloadQueue(root, queueEntries(pending)); err != nil {
			return nil, err
		}
	}
	return pending, nil
}

func writeDownloadQueue(root string, entries []savedDownload) error {
	data, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	if len(data) > maxQueueBytes {
		return fmt.Errorf("下载队列文件过大")
	}
	return storage.WriteFile(filepath.Join(root, "download-queue.json"), data)
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

func (a *desktop) reloadDownloadQueue() {
	q := &a.queue
	if q.loading {
		return
	}
	q.loading = true
	history, root := slices.Clone(a.history), a.store.Root
	go func() {
		items, err := readDownloadQueue(root, history)
		a.post(func() {
			q.loading = false
			q.loadError = err
			if err == nil {
				q.pending = items
			}
		})
	}()
}

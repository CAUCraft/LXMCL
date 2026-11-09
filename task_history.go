package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	historyLimit = 24
	logTailLimit = 64 << 10

	taskCompleted = "已完成"
	taskStarted   = "已启动"
	taskExited    = "已退出"
	taskFailed    = "已失败"
	taskCanceled  = "已取消"
)

type taskRecord struct {
	Name    string    `json:"name"`
	Version string    `json:"version"`
	Action  string    `json:"action"`
	Status  string    `json:"status"`
	Detail  string    `json:"detail,omitempty"`
	Log     string    `json:"log,omitempty"`
	PID     int       `json:"pid,omitempty"`
	At      time.Time `json:"at"`
}

func loadTaskHistory(root string) ([]taskRecord, error) {
	f, err := os.Open(filepath.Join(root, "tasks.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var records []taskRecord
	if err := json.NewDecoder(io.LimitReader(f, 2<<20)).Decode(&records); err != nil {
		return nil, err
	}
	return records[:min(len(records), historyLimit)], nil
}

func updateTaskHistory(records []taskRecord, record taskRecord) []taskRecord {
	next := []taskRecord{record}
	for _, item := range records {
		if !item.At.Equal(record.At) {
			next = append(next, item)
		}
		if len(next) == historyLimit {
			break
		}
	}
	return next
}

func saveTaskHistory(root string, records []taskRecord) error {
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

func (a *desktop) restoreTaskHistory(records []taskRecord) {
	a.history = records
	for _, r := range records {
		if isGameLog(r.Log) {
			a.lastLog = r.Log
			break
		}
	}
}

func (a *desktop) saveTask(records []taskRecord, record taskRecord, render func()) []taskRecord {
	next := updateTaskHistory(records, record)
	err := saveTaskHistory(a.store.Root, next)
	a.post(func() {
		a.history = next
		render()
		if err != nil {
			// Preserve the game outcome and report the separate disk error.
			a.notice += " · 任务记录保存失败：" + err.Error()
			a.noticeError = true
		}
	})
	return next
}

func (a *desktop) taskSnapshot() []taskRecord { return slices.Clone(a.history) }

func isGameLog(path string) bool {
	base := filepath.Base(path)
	return filepath.IsAbs(path) && filepath.Base(filepath.Dir(path)) == "launcher-logs" &&
		strings.HasPrefix(base, "game-") && filepath.Ext(base) == ".log"
}

func readGameLog(path string) (string, error) {
	if !isGameLog(path) {
		return "", errors.New("无效的游戏日志路径")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !st.Mode().IsRegular() {
		return "", errors.New("日志不是普通文件")
	}
	offset := max(st.Size()-logTailLimit, 0)
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return "", err
	}
	b, err := io.ReadAll(io.LimitReader(f, logTailLimit))
	if err != nil {
		return "", err
	}
	if offset != 0 {
		if _, tail, ok := strings.Cut(string(b), "\n"); ok {
			b = []byte(tail)
		}
	}
	log := strings.ToValidUTF8(string(b), "\uFFFD")
	if log == "" {
		return "日志暂时为空。", nil
	}
	if offset != 0 {
		return "仅显示日志末尾 64 KiB。\n\n" + log, nil
	}
	return log, nil
}

func (a *desktop) showLog(path string) {
	if !isGameLog(path) {
		return
	}
	a.logRequest++
	request := a.logRequest
	a.logPath, a.logText, a.logOpen = path, "正在读取游戏日志…", true
	go func() {
		log, err := readGameLog(path)
		a.post(func() {
			if a.logRequest != request {
				return
			}
			if err != nil {
				a.logText = fmt.Sprintf("无法读取日志：%s", err)
			} else {
				a.logText = log
			}
		})
	}()
}

func (a *desktop) openLog(path string) {
	if isGameLog(path) {
		a.openPath(path, false)
	}
}

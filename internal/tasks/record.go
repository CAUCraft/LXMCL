package tasks

import (
	"context"
	"crypto/rand"
	"errors"
	"strconv"
	"time"
)

// Persisted values retain the original tasks.json format. Do not translate these
// discriminators; presentation labels belong to the caller.
const (
	Limit     = 24
	Completed = "已完成"
	Started   = "已启动"
	Exited    = "已退出"
	Failed    = "已失败"
	Canceled  = "已取消"
	Launch    = "启动游戏"
	Verify    = "下载 / 校验"
)

type Resource struct {
	// Kind is the persisted resource identifier. The operation that retries a
	// record interprets it and checks support for the submitted instance.
	Kind    string `json:"kind"`
	Project string `json:"project"`
	Version string `json:"version"`
	Title   string `json:"title"`
}

type Record struct {
	ID string `json:"id,omitempty"`

	InstanceID int       `json:"instance_id,omitempty"`
	Revision   uint64    `json:"instance_revision,omitempty"`
	Name       string    `json:"name"`
	Version    string    `json:"version"`
	Server     string    `json:"server,omitempty"`
	Account    string    `json:"account,omitempty"`
	Action     string    `json:"action"`
	Status     string    `json:"status"`
	Detail     string    `json:"detail,omitempty"`
	Folder     string    `json:"folder,omitempty"`
	Log        string    `json:"log,omitempty"`
	PID        int       `json:"pid,omitempty"`
	At         time.Time `json:"at"`
	Resource   *Resource `json:"resource,omitempty"`
}

func (r Record) Key() string {
	if r.ID != "" {
		return r.ID
	}
	return strconv.FormatInt(r.At.UnixNano(), 10)
}

func (r Record) Submitted() Record {
	if r.ID == "" {
		r.ID = rand.Text()
	}
	if r.At.IsZero() {
		r.At = time.Now()
	}
	if r.Resource != nil {
		copy := *r.Resource
		r.Resource = &copy
	}
	return r
}

func Outcome(err error) string {
	if errors.Is(err, context.Canceled) {
		return Canceled
	}
	if err != nil {
		return Failed
	}
	return Completed
}

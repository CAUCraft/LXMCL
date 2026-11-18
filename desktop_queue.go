package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/layout"
)

type queuedResource struct {
	id       string
	target   launcher.Instance
	folder   string
	resource resourceTask
	version  launcher.PackVersion
}
type resourceQueue struct {
	pending              []queuedResource
	running              bool
	activeID             string
	lastWrite            chan struct{}
	loading              bool
	loadError, saveError error
	scroll               ui.ScrollState
}

func (a *desktop) enqueueResource(item queuedResource) {
	if a.queue.loadError != nil {
		a.message("请在下载任务中重新读取队列", a.queue.loadError)
		return
	}
	if a.ctx == nil || !a.ready {
		return
	}
	if slices.ContainsFunc(a.queue.pending, func(other queuedResource) bool {
		return other.target.ID == item.target.ID && other.target.Revision == item.target.Revision && other.folder == item.folder && other.resource.Project == item.resource.Project && other.resource.Version == item.resource.Version
	}) {
		a.message("此版本已在队列中", nil)
		return
	}
	item.id = rand.Text()
	a.queue.pending = append(a.queue.pending, item)
	a.saveDownloadQueue()
	a.message("已加入下载队列 · "+item.resource.Title, nil)
}
func (a *desktop) queuePanel(c *ui.Context) {
	q := &a.queue
	if q.loadError != nil {
		feedback.Alert(c, "读取下载队列失败："+q.loadError.Error(), feedback.AlertProps{Type: feedback.Error})
		if button(c, "重新读取队列", action.Default, q.loading) {
			a.reloadDownloadQueue()
		}
		return
	}
	if q.saveError != nil {
		feedback.Alert(c, "保存下载队列失败："+q.saveError.Error(), feedback.AlertProps{Type: feedback.Error})
		if button(c, "重新保存队列", action.Default, q.lastWrite != nil) {
			a.saveDownloadQueue()
		}
	}
	pending := q.pending
	if q.activeID != "" && len(pending) > 0 {
		pending = pending[1:]
	}
	if len(pending) == 0 {
		return
	}
	sectionPane(c).Padding(8, 12).Gap(4).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			display.Text(c, fmt.Sprintf("下载队列 · 待处理 %d", len(pending)), display.TextProps{Strong: true}).Grow(1)
			if q.running {
				if button(c, "完成当前项后暂停", action.Default, false) {
					q.running = false
				}
			} else if button(c, "开始队列", action.Primary, a.busy || a.running != nil || a.ctx == nil || q.saveError != nil || q.lastWrite != nil) {
				q.running = true
				a.nextResource()
			}
		})
		remove := -1
		layout.ScrollArea(c, layout.ScrollAreaProps{State: &q.scroll}, func() {
			for n, item := range pending {
				ui.Row(c.Key(item.id + item.resource.Version)).Gap(8).AlignItems(ui.Center).Children(func() {
					dest := item.folder
					if item.target.ID != 0 {
						dest = item.target.Name
					}
					display.Text(c, item.resource.Title+" "+item.version.Number+" → "+dest, display.TextProps{Rows: 1}).Grow(1).MinWidth(0).Ellipsis("…")
					if button(c, "移出", action.Text, false) {
						remove = n
					}
				})
			}
		}).Height(min(float32(100), float32(34*len(pending))))
		if remove >= 0 {
			id := pending[remove].id
			if id != q.activeID {
				q.pending = slices.DeleteFunc(q.pending, func(item queuedResource) bool { return item.id == id })
				a.saveDownloadQueue()
			}
		}
	})
}

func (a *desktop) nextResource() {
	q := &a.queue
	if !q.running || q.activeID != "" || q.lastWrite != nil {
		return
	}
	if len(q.pending) == 0 || a.busy || a.running != nil || a.ctx == nil || a.ctx.Err() != nil {
		q.running = false
		return
	}
	item := q.pending[0]
	q.activeID = item.id
	a.runResource(item, func(err error) {
		q.activeID = ""
		if errors.Is(err, context.Canceled) {
			q.running = false
		}
		q.pending = slices.Delete(q.pending, 0, 1)
		a.saveDownloadQueue()
	})
}

func (a *desktop) runResource(item queuedResource, done func(error)) {
	if a.ctx == nil || a.busy || a.running != nil {
		return
	}
	i, r, v := item.target, item.resource, item.version
	kind := launcher.ContentKind(r.Kind)
	row := taskRecord{ID: item.id, Folder: item.folder, Name: r.Title, Action: "保存" + contentLabel(kind), Resource: &r}
	current, found := a.state.Find(i.ID)
	if i.ID != 0 {
		row.InstanceID, row.Revision, row.Name, row.Version, row.Action = i.ID, i.Revision, i.Name, i.Version, "安装"+contentLabel(kind)
	}
	game := a.game
	a.runFileTask(row, func(ctx context.Context, report func(launcher.Progress)) error {
		if i.ID != 0 && (!found || current.Revision != i.Revision || current.Version != i.Version || current.GameDir != i.GameDir || !kind.Supports(current)) {
			return errors.New("实例已变更，请重新选择兼容文件")
		}
		if i.ID == 0 && !filepath.IsAbs(item.folder) {
			return errors.New("请选择下载目录")
		}
		if v.ID == "" {
			var err error
			v, err = game.ModrinthVersion(ctx, r.Project, r.Version)
			if err != nil {
				return err
			}
		}
		if i.ID != 0 {
			return game.DownloadModrinth(ctx, current, kind, v, report)
		}
		if item.id != "" && item.version.ID == "" {
			return game.ResumeResourceFile(ctx, kind, v, item.folder, report)
		}
		return game.DownloadResourceFile(ctx, kind, v, item.folder, report)
	}, func(err error) {
		if i.ID != 0 && a.content.id == i.ID && a.content.revision == i.Revision {
			a.refreshContent(i)
		}
		if done != nil {
			done(err)
		}
	})
}

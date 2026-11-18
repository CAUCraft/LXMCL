package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/layout"
)

type queuedResource struct {
	target   launcher.Instance
	folder   string
	resource resourceTask
	version  launcher.PackVersion
}
type resourceQueue struct {
	pending []queuedResource
	running bool
	scroll  ui.ScrollState
}

func (a *desktop) enqueueResource(item queuedResource) {
	if slices.ContainsFunc(a.queue.pending, func(other queuedResource) bool {
		return other.target.ID == item.target.ID && other.target.Revision == item.target.Revision && other.folder == item.folder && other.resource.Project == item.resource.Project && other.resource.Version == item.resource.Version
	}) {
		a.message("此版本已在队列中", nil)
		return
	}
	a.queue.pending = append(a.queue.pending, item)
	a.message("已加入下载队列 · "+item.resource.Title, nil)
}
func (a *desktop) queuePanel(c *ui.Context) {
	q := &a.queue
	if len(q.pending) == 0 {
		return
	}
	sectionPane(c).Padding(8, 12).Gap(4).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			display.Text(c, fmt.Sprintf("下载队列 · 待处理 %d", len(q.pending)), display.TextProps{Strong: true}).Grow(1)
			if q.running {
				if button(c, "完成当前项后暂停", action.Default, false) {
					q.running = false
				}
			} else if button(c, "开始队列", action.Primary, a.busy || a.running != nil || a.ctx == nil) {
				q.running = true
				a.nextResource()
			}
		})
		remove := -1
		layout.ScrollArea(c, layout.ScrollAreaProps{State: &q.scroll}, func() {
			for n, item := range q.pending {
				ui.Row(c.Key(n)).Gap(8).AlignItems(ui.Center).Children(func() {
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
		}).Height(min(float32(100), float32(34*len(q.pending))))
		if remove >= 0 {
			q.pending = slices.Delete(q.pending, remove, remove+1)
		}
	})
}

func (a *desktop) nextResource() {
	q := &a.queue
	if !q.running || len(q.pending) == 0 {
		q.running = false
		return
	}
	item := q.pending[0]
	q.pending = q.pending[1:]
	a.runResource(item, func(err error) {
		if errors.Is(err, context.Canceled) {
			q.running = false
		}
		a.nextResource()
	})
}

func (a *desktop) runResource(item queuedResource, done func(error)) {
	if a.ctx == nil || a.busy || a.running != nil {
		return
	}
	i, r, v := item.target, item.resource, item.version
	kind := launcher.ContentKind(r.Kind)
	row := taskRecord{Folder: item.folder, Name: r.Title, Action: "保存" + contentLabel(kind), Resource: &r}
	current, found := a.state.Find(i.ID)
	if i.ID != 0 {
		row.InstanceID, row.Revision, row.Name, row.Version, row.Action = i.ID, i.Revision, i.Name, i.Version, "安装"+contentLabel(kind)
	}
	game := a.game
	a.runFileTask(row, func(ctx context.Context, report func(launcher.Progress)) error {
		if i.ID != 0 && (!found || current.Revision != i.Revision || !kind.Supports(i)) {
			return errors.New("实例已变更，请重新选择兼容文件")
		}
		if i.ID == 0 && !filepath.IsAbs(item.folder) {
			return errors.New("请选择下载目录")
		}
		if v.ID == "" {
			var err error
			v, err = launcher.ModrinthVersion(ctx, r.Project, r.Version)
			if err != nil {
				return err
			}
		}
		if i.ID != 0 {
			return game.DownloadModrinth(ctx, i, kind, v, report)
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

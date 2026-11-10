package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/layout"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

type contentView struct {
	id, tab  int
	revision uint64
	files    []launcher.ContentFile
	reading  bool
	err      string
}

func (v contentView) kind() launcher.ContentKind {
	switch v.tab {
	case 1:
		return launcher.Mods
	case 2:
		return launcher.ShaderPacks
	default:
		return launcher.ResourcePacks
	}
}

func (a *desktop) refreshContent(i launcher.Instance) {
	if a.ctx == nil {
		return
	}
	a.contentRequest++
	request, kind := a.contentRequest, a.content.kind()
	a.content.reading, a.content.err, a.content.files = true, "", nil
	go func() {
		files, err := launcher.ListContent(i, kind)
		a.post(func() {
			if request == a.contentRequest {
				a.content.reading, a.content.files = false, files
				if err != nil {
					a.content.err = err.Error()
				}
			}
		})
	}()
}

func (a *desktop) instanceContent(c *ui.Context, i launcher.Instance) {
	v := &a.content
	if v.id != i.ID || v.revision != i.Revision || v.tab != 0 && !v.kind().Supports(i) {
		*v = contentView{id: i.ID, revision: i.Revision}
		a.refreshContent(i)
	}
	disabled := a.busy || a.pickerBusy || a.running != nil
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		if launcher.Mods.Supports(i) {
			old := v.tab
			options := []string{"资源包", "模组"}
			if launcher.ShaderPacks.Supports(i) {
				options = append(options, "光影包")
			}
			action.Segmented(c, &v.tab, options, action.SegmentedProps{Size: theme.Small}).Label("资源类型")
			if old != v.tab {
				a.refreshContent(i)
			}
		} else {
			display.Text(c, "资源包", display.TextProps{Strong: true})
		}
		ui.Spacer(c).Grow(1)
		kind := v.kind()
		if button(c, "下载"+contentLabel(kind)+"…", action.Primary, false) {
			a.openResourceDownloads(i, kind)
		}
		if button(c, "导入…", action.Default, disabled) {
			ext := "zip"
			if kind == launcher.Mods {
				ext = "jar"
			}
			a.pickPath(mygo.OpenDialogOptions{Title: "选择本地资源", Filters: []mygo.FileFilter{{Name: "游戏资源", Extensions: []string{ext}}}}, func(path string) {
				a.runInstanceTask(i, "导入资源", func(ctx context.Context) error {
					return launcher.ImportContent(ctx, i, kind, path)
				})
			})
		}
		if icon(c, lucide.RefreshCw(), "刷新资源", v.reading) {
			a.refreshContent(i)
		}
		if icon(c, lucide.FolderOpen(), "打开"+contentLabel(kind)+"目录", false) {
			a.openPath(filepath.Join(i.GameDir, string(kind)), true)
		}
	})
	switch {
	case v.reading:
		flightLoading(c, "正在读取资源…")
	case v.err != "":
		feedback.Alert(c, v.err, feedback.AlertProps{Type: feedback.Error})
	case len(v.files) == 0:
		emptyPanel(c, lucide.PackageOpen(), "暂无本地资源")
	default:
		layout.ScrollArea(c, layout.ScrollAreaProps{}, func() {
			sectionPane(c).Padding(5, 12).Gap(4).Children(func() {
				for idx, file := range v.files {
					if idx > 0 {
						layout.Divider(c, layout.DividerProps{})
					}
					ui.Row(c.Key(file.Name + fmt.Sprint(file.Enabled))).MinHeight(42).Gap(8).AlignItems(ui.Center).Children(func() {
						display.Text(c, file.Name, display.TextProps{Strong: true, Rows: 1}).Grow(1).MinWidth(0).Ellipsis("…")
						muted(c, fmt.Sprintf("%.1f MiB", float64(file.Size)/(1<<20))).FontSize(12)
						label := "停用"
						if !file.Enabled {
							label = "恢复"
							display.Tag(c, "已停用", display.TagProps{Color: display.TagGray, Size: theme.Small})
						}
						if button(c, label, action.Default, disabled) {
							kind := v.kind()
							a.runInstanceTask(i, label+"资源", func(context.Context) error {
								return launcher.SetContentEnabled(i, kind, file.Name, !file.Enabled)
							})
						}
					})
				}
			})
		}).Grow(1).MinHeight(0)
	}
}

func (a *desktop) runInstanceTask(i launcher.Instance, label string, work func(context.Context) error) {
	a.runContentTask(i, label, nil, func(ctx context.Context, _ func(launcher.Progress)) error { return work(ctx) })
}

func (a *desktop) runContentTask(i launcher.Instance, label string, resource *resourceTask, work func(context.Context, func(launcher.Progress)) error) {
	current, ok := a.state.Find(i.ID)
	if !ok || current.Revision != i.Revision || a.busy || a.running != nil || a.ctx == nil {
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.busy, a.taskName, a.cancelTask = true, i.Name, cancel
	a.task, a.notice = launcher.Progress{Stage: label}, ""
	history := a.taskSnapshot()
	row := taskRecord{InstanceID: i.ID, Revision: i.Revision, Name: i.Name, Version: i.Version, Action: label, At: time.Now(), Resource: resource}
	a.writes.Go(func() {
		defer cancel()
		err := work(ctx, func(p launcher.Progress) { a.post(func() { a.task = p }) })
		row.Status = taskCompleted
		if errors.Is(err, context.Canceled) {
			row.Status = taskCanceled
		} else if err != nil {
			row.Status, row.Detail = taskFailed, err.Error()
		}
		a.saveTask(history, row, func() {
			a.busy, a.cancelTask, a.task = false, nil, launcher.Progress{}
			a.message(label+" · "+row.Status, err)
			a.noticeTask = err != nil
			if a.content.id == i.ID && a.content.revision == i.Revision {
				a.refreshContent(i)
			}
		})
	})
}

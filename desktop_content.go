package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/field"
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
	if v.tab == 1 {
		return launcher.FabricMods
	}
	return launcher.ResourcePacks
}

func (a *desktop) refreshContent(i launcher.Instance) {
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
	if v.id != i.ID || v.revision != i.Revision {
		*v = contentView{id: i.ID, revision: i.Revision}
		a.refreshContent(i)
	}
	disabled := a.busy || a.pickerBusy || a.running != nil
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		if i.Fabric() {
			old := v.tab
			action.Segmented(c, &v.tab, []string{"资源包", "Fabric 模组"}, action.SegmentedProps{Size: theme.Small}).Label("资源类型")
			if old != v.tab {
				a.refreshContent(i)
			}
		} else {
			display.Text(c, "资源包", display.TextProps{Strong: true})
		}
		ui.Spacer(c).Grow(1)
		kind := v.kind()
		if button(c, "搜索 Modrinth", action.Default, disabled) {
			a.pack = &packBrowser{open: true, target: i, kind: kind}
		}
		if button(c, "导入本地文件…", action.Primary, disabled) {
			ext := "zip"
			if kind == launcher.FabricMods {
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
	current, ok := a.state.Find(i.ID)
	if !ok || current.Revision != i.Revision || a.busy || a.running != nil {
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.busy, a.taskName, a.cancelTask = true, i.Name, cancel
	a.task, a.notice = launcher.Progress{Stage: label}, ""
	history := a.taskSnapshot()
	row := taskRecord{InstanceID: i.ID, Revision: i.Revision, Name: i.Name, Version: i.Version, Action: label, At: time.Now()}
	a.writes.Go(func() {
		defer cancel()
		err := work(ctx)
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

type packBrowser struct {
	open, busy, searched bool
	target               launcher.Instance
	kind                 launcher.ContentKind
	query, err, version  string
	hits                 []launcher.PackProject
	project              launcher.PackProject
	versions             []launcher.PackVersion
}

func (a *desktop) searchPacks(p *packBrowser) {
	p.busy, p.err, p.searched = true, "", true
	p.project, p.version, p.versions = launcher.PackProject{}, "", nil
	query := p.query
	go func() {
		ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
		defer cancel()
		hits, err := launcher.SearchModrinth(ctx, p.target.Version, p.kind, query)
		a.post(func() {
			p.busy, p.hits = false, hits
			if err != nil {
				p.err = err.Error()
			}
		})
	}()
}

func (a *desktop) browsePack(p *packBrowser, project launcher.PackProject) {
	p.project, p.version, p.versions, p.err, p.busy = project, "", nil, "", true
	go func() {
		ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
		defer cancel()
		versions, err := launcher.ModrinthVersions(ctx, project.ID, p.target.Version, p.kind)
		a.post(func() {
			p.busy, p.versions = false, versions
			if err != nil {
				p.err = err.Error()
			} else if len(versions) > 0 {
				p.version = versions[0].ID
			}
		})
	}()
}

func (a *desktop) packDialog(c *ui.Context) {
	p := a.pack
	if p == nil || !p.open {
		return
	}
	label := "资源包"
	if p.kind == launcher.FabricMods {
		label = "模组"
	}
	feedback.Modal(c, &p.open, feedback.ModalProps{Title: "Modrinth " + label, Width: 720, NoFooter: true}, func() {
		ui.Column(c).Gap(9).Children(func() {
			panelHeading(c, lucide.PackageSearch(), p.target.Name, "Minecraft "+p.target.Version)
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				submit := false
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					submit = field.Input(c, &p.query, field.InputProps{Placeholder: "搜索兼容" + label}).
						Label(label + "关键词").Disabled(p.busy).Submitted()
				})
				if button(c, "搜索", action.Primary, p.busy) || submit && !p.busy {
					a.searchPacks(p)
				}
			})
			ui.Column(c).Height(240).Children(func() {
				splitWorkspace(c, 215, func() {
					layout.ScrollArea(c, layout.ScrollAreaProps{}, func() {
						items := make([]display.ListItem, 0, len(p.hits))
						for _, hit := range p.hits {
							items = append(items, display.ListItem{Key: hit.ID, Label: hit.Title, Description: hit.Author, Disabled: p.busy})
						}
						if len(items) == 0 {
							muted(c, "暂无搜索结果").Padding(4)
							return
						}
						id := display.List(c, display.ListProps{Items: items, Active: p.project.ID, Compact: true})
						for _, hit := range p.hits {
							if hit.ID == id && id != p.project.ID {
								a.browsePack(p, hit)
								c.Invalidate()
								break
							}
						}
					}).Grow(1).MinHeight(0)
				}, func() {
					detailPane(c.Key(p.project.ID), func() { a.packDetails(c, p, label) })
				})
			})
		})
	})
}

func (a *desktop) packDetails(c *ui.Context, p *packBrowser, label string) {
	if p.project.ID != "" {
		panelHeading(c, lucide.Package(), p.project.Title, p.project.Author)
	}
	switch {
	case p.busy:
		flightLoading(c, "正在读取 Modrinth…")
	case p.err != "":
		feedback.Alert(c, p.err, feedback.AlertProps{Type: feedback.Error})
		if p.project.ID != "" && button(c, "重试读取版本", action.Default, false) {
			a.browsePack(p, p.project)
		}
	case p.project.ID == "":
		title := "选择" + label + "查看详情"
		if len(p.hits) == 0 {
			title = "搜索兼容的" + label
			if p.searched {
				title = "没有匹配的" + label
			}
		}
		display.Empty(c, display.EmptyProps{Icon: lucide.PackageSearch(), Title: title})
	case len(p.versions) == 0:
		muted(c, "没有可用的兼容版本")
	default:
		options := make([]field.SelectOption, 0, len(p.versions))
		for _, v := range p.versions {
			options = append(options, field.SelectOption{Value: v.ID, Label: v.Number})
		}
		catalogPicker(c, &p.version, options, label+"版本", "", false)
		if p.kind == launcher.FabricMods {
			muted(c, "会同时下载必要依赖。换版本前请停用旧模组。")
		} else {
			muted(c, "下载后可在游戏内启用资源包。")
		}
		if button(c, "下载"+label, action.Primary, p.version == "" || a.busy || a.running != nil) {
			for _, version := range p.versions {
				if version.ID == p.version {
					p.open = false
					a.runInstanceTask(p.target, "下载"+label, func(ctx context.Context) error {
						return launcher.DownloadModrinth(ctx, p.target, p.kind, version)
					})
					break
				}
			}
		}
	}
}

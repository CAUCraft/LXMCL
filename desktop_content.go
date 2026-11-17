package main

import (
	"cmp"
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/CAUCraft/LXMCL/internal/tasks"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/choice"
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
	query    string
	filter   int
	order    string
	selected map[string]bool // physical filename → enabled state
	scroll   ui.ListState
	listing  *resourceListing
}

type resourceListing struct {
	query     string
	filter    int
	files     []launcher.ContentFile
	clipboard string
}

// Build the clipboard text and filtered view when the listing changes, including
// a new disk scan or sort. Scrolling and hover frames reuse the same snapshot.
func (v *contentView) currentListing() *resourceListing {
	if v.listing != nil && v.listing.query == v.query && v.listing.filter == v.filter {
		return v.listing
	}
	list := &resourceListing{query: v.query, filter: v.filter, files: v.visibleFiles()}
	var text strings.Builder
	for _, file := range list.files {
		if file.Enabled {
			text.WriteString("[启用] ")
		} else {
			text.WriteString("[停用] ")
		}
		text.WriteString(file.Name)
		text.WriteByte('\n')
	}
	list.clipboard = text.String()
	v.listing = list
	return list
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
	a.content.selected, a.content.listing = nil, nil
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
				a.content.sortFiles()
				if err != nil {
					a.content.err = err.Error()
				}
			}
		})
	}()
}

func (a *desktop) inspectContent(i launcher.Instance, kind launcher.ContentKind) {
	a.inspect(i)
	a.detailPage = detailContent
	tab := 0
	if kind.Supports(i) {
		switch kind {
		case launcher.Mods:
			tab = 1
		case launcher.ShaderPacks:
			tab = 2
		}
	}
	a.content = contentView{id: i.ID, revision: i.Revision, tab: tab}
	a.refreshContent(i)
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
				v.query, v.filter, v.scroll = "", 0, ui.ListState{}
				a.refreshContent(i)
			}
		} else {
			display.Text(c, "资源包", display.TextProps{Strong: true})
		}
		ui.Spacer(c).Grow(1)
		kind := v.kind()
		if button(c, "添加"+contentLabel(kind)+"…", action.Primary, false) {
			a.openResourceDownloads(i, kind)
		}
		if button(c, "导入…", action.Default, disabled) {
			ext := "zip"
			if kind == launcher.Mods {
				ext = "jar"
			}
			a.pickPaths(mygo.OpenDialogOptions{Title: "选择本地资源（可多选）", Multiple: true,
				Filters: []mygo.FileFilter{{Name: "游戏资源", Extensions: []string{ext}}}}, func(paths []string) {
				a.runInstanceTask(i, fmt.Sprintf("导入 %d 项资源", len(paths)), func(ctx context.Context) error {
					completed, err := launcher.ImportContents(ctx, i, kind, paths)
					if err != nil {
						return fmt.Errorf("已导入 %d / %d 项：%w", completed, len(paths), err)
					}
					return nil
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
		a.contentList(c, i, disabled)
	}
}

func contentKey(file launcher.ContentFile) string {
	if file.Enabled {
		return file.Name
	}
	return file.Name + ".disabled"
}

func (v *contentView) visibleFiles() []launcher.ContentFile {
	query := strings.ToLower(strings.TrimSpace(v.query))
	if query == "" && v.filter == 0 {
		return v.files
	}
	var files []launcher.ContentFile
	for _, file := range v.files {
		if query != "" && !strings.Contains(strings.ToLower(file.Name), query) ||
			v.filter == 1 && !file.Enabled || v.filter == 2 && file.Enabled {
			continue
		}
		files = append(files, file)
	}
	return files
}

func (v *contentView) sortFiles() {
	v.listing = nil
	slices.SortFunc(v.files, func(a, b launcher.ContentFile) int {
		switch v.order {
		case "modified":
			if n := b.Modified.Compare(a.Modified); n != 0 {
				return n
			}
		case "size":
			if n := cmp.Compare(b.Size, a.Size); n != 0 {
				return n
			}
		}
		if n := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); n != 0 {
			return n
		}
		if a.Name == b.Name && a.Enabled != b.Enabled {
			if a.Enabled {
				return 1
			}
			return -1
		}
		return strings.Compare(a.Name, b.Name)
	})
}

func (v *contentView) selection(files []launcher.ContentFile) []launcher.ContentFile {
	if len(v.selected) == 0 {
		return nil
	}
	var selected []launcher.ContentFile
	for _, file := range files {
		if _, ok := v.selected[contentKey(file)]; ok {
			selected = append(selected, file)
		}
	}
	return selected
}

func (a *desktop) contentList(c *ui.Context, i launcher.Instance, disabled bool) {
	v := &a.content
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		oldQuery, oldFilter := v.query, v.filter
		field.SearchBar(c, &v.query, field.SearchBarProps{Placeholder: "搜索本地文件"}).Label("搜索本地资源").Grow(1).MinWidth(0)
		action.Segmented(c, &v.filter, []string{"全部资源", "已启用", "已停用"}, action.SegmentedProps{Size: theme.Small}).Label("本地资源筛选")
		if oldQuery != v.query || oldFilter != v.filter {
			v.selected, v.scroll = nil, ui.ListState{}
		}
	})
	listing := v.currentListing()
	files := listing.files
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		n := len(v.selected)
		all := n > 0 && n == len(files)
		if choice.Checkbox(c, &all, "全选当前列表", choice.CheckboxProps{Indeterminate: n > 0 && n < len(files)}).
			Disabled(disabled || len(files) == 0).Changed() {
			v.selected = make(map[string]bool, len(files))
			if all {
				for _, file := range files {
					v.selected[contentKey(file)] = file.Enabled
				}
			}
		}
		muted(c, fmt.Sprintf("%d / %d 项", len(files), len(v.files))).FontSize(12).Grow(1)
		order := v.order
		ui.Column(c).Width(124).Shrink(0).Children(func() {
			field.Select(c, &v.order, field.SelectProps{Options: []field.SelectOption{
				{Value: "", Label: "按名称"}, {Value: "modified", Label: "最近修改"}, {Value: "size", Label: "文件较大"},
			}}).Label("本地资源排序")
		})
		if order != v.order {
			v.scroll = ui.ListState{}
			v.sortFiles()
			listing = v.currentListing()
			files = listing.files
		}
		action.CopyButton(c, listing.clipboard, action.CopyButtonProps{}).Label("复制资源列表").Disabled(len(files) == 0)
	})
	if len(files) == 0 {
		ui.Column(c).Grow(1).Center().Children(func() {
			display.Empty(c, display.EmptyProps{Icon: lucide.Search(), Title: "没有匹配的资源", Action: func() {
				if button(c, "清除资源筛选", action.Default, false) {
					v.query, v.filter = "", 0
				}
			}})
		})
		return
	}
	ui.List(c, &v.scroll, len(files)).ItemKey(func(row int) any {
		return contentKey(files[row])
	}).Grow(1).MinHeight(0).Rows(func(row ui.ListRow) {
		c, file := row.Context, files[row.Index]
		if row.Index > 0 {
			layout.Divider(c, layout.DividerProps{})
		}
		key := contentKey(file)
		ui.Row(c).MinHeight(46).Padding(0, 12).Gap(8).AlignItems(ui.Center).Children(func() {
			_, checked := v.selected[key]
			if choice.Checkbox(c, &checked, "", choice.CheckboxProps{}).Label("选择资源 " + file.Name).
				Disabled(disabled).Changed() {
				if v.selected == nil {
					v.selected = make(map[string]bool)
				}
				if checked {
					v.selected[key] = file.Enabled
				} else {
					delete(v.selected, key)
				}
			}
			leadIcon(c, downloadTabs[downloadTab(v.kind())].Icon, 26)
			display.Text(c, file.Name, display.TextProps{Strong: true, Rows: 1}).Grow(1).MinWidth(0).Ellipsis("…")
			detail := fileSizeLabel(file.Size)
			if v.order == "modified" && !file.Modified.IsZero() {
				detail = file.Modified.Local().Format("01-02 15:04") + " · " + detail
			}
			muted(c, detail).FontSize(12)
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
	})
	if count := len(v.selected); count > 0 {
		enabled := 0
		for _, active := range v.selected {
			if active {
				enabled++
			}
		}
		layout.Divider(c, layout.DividerProps{})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			muted(c, fmt.Sprintf("已选 %d 项", count)).FontSize(12).Grow(1)
			if button(c, "取消选择", action.Text, false) {
				v.selected = nil
			}
			if button(c, "启用所选", action.Default, disabled || enabled == count) {
				a.setSelectedContent(i, v.kind(), v.selection(files), true)
			}
			if button(c, "停用所选", action.Default, disabled || enabled == 0) {
				a.setSelectedContent(i, v.kind(), v.selection(files), false)
			}
		})
	}
}

func (a *desktop) setSelectedContent(i launcher.Instance, kind launcher.ContentKind, files []launcher.ContentFile, enabled bool) {
	label := "批量停用资源"
	if enabled {
		label = "批量启用资源"
	}
	a.runInstanceTask(i, label, func(ctx context.Context) error {
		changed, err := launcher.SetContentEnabledBatch(ctx, i, kind, files, enabled)
		if err != nil {
			return fmt.Errorf("已更改 %d 项，其余资源请刷新后检查：%w", changed, err)
		}
		return nil
	})
}

func (a *desktop) runInstanceTask(i launcher.Instance, label string, work func(context.Context) error) {
	a.runContentTask(i, label, nil, func(ctx context.Context, _ func(launcher.Progress)) error { return work(ctx) })
}

func (a *desktop) runContentTask(i launcher.Instance, label string, resource *resourceTask, work func(context.Context, func(launcher.Progress)) error) {
	current, ok := a.state.Find(i.ID)
	if !ok || current.Revision != i.Revision || a.busy || a.running != nil || a.ctx == nil {
		return
	}
	row := taskRecord{InstanceID: i.ID, Revision: i.Revision, Name: i.Name, Version: i.Version, Action: label, At: time.Now(), Resource: resource}
	a.runFileTask(row, work, func() {
		if label == "备份存档" && a.backups != nil && a.backups.instanceID == i.ID {
			a.refreshBackups(i.ID)
		}
		if a.content.id == i.ID && a.content.revision == i.Revision {
			a.refreshContent(i)
		}
	})
}

func (a *desktop) runFolderDownload(folder, action, name string, resource *resourceTask, work func(context.Context, func(launcher.Progress)) error) {
	if a.busy || a.running != nil || a.ctx == nil || !filepath.IsAbs(folder) {
		return
	}
	row := taskRecord{Name: name, Action: action, Folder: folder, At: time.Now(), Resource: resource}
	a.runFileTask(row, work, nil)
}

func (a *desktop) runFileTask(row taskRecord, work func(context.Context, func(launcher.Progress)) error, refresh func()) {
	row = row.Submitted()
	ctx, cancel := context.WithCancel(a.ctx)
	a.busy, a.taskName, a.cancelTask = true, row.Name, cancel
	a.taskCanceling = false
	a.task, a.notice = launcher.Progress{Stage: row.Action}, ""
	a.writes.Go(func() {
		defer cancel()
		err := tasks.Run(ctx, work, func(p launcher.Progress) { a.post(func() { a.task = p }) })
		row.Status = tasks.Outcome(err)
		if err != nil {
			row.Detail = err.Error()
		}
		a.saveTask(row, func() {
			a.busy, a.cancelTask, a.task = false, nil, launcher.Progress{}
			a.taskCanceling = false
			a.message(row.Action+" · "+row.Status, err)
			a.noticeTask = err != nil
			if refresh != nil {
				refresh()
			}
		})
	})
}

func fileSizeLabel(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1f GiB", float64(bytes)/(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(bytes)/(1<<10))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

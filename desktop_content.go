package main

import (
	"cmp"
	"context"
	"errors"
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
	savedPacks  *savedPacksView
	packPreview *resourcePackView
	comparison  *modComparisonView
	modNotes    *modReleaseNotesView
	restoreMods *modRestoreView
	copyMods    *modCopyView
	mods        map[string]launcher.InstalledMod
	checking    bool
	checkError  string
	id, tab     int
	revision    uint64
	files       []launcher.ContentFile
	reading     bool
	err         string
	query       string
	filter      int
	order       string
	selected    map[string]bool // physical filename → enabled state
	scroll      ui.ListState
	listing     *resourceListing
}

type resourceListing struct {
	query      string
	filter     int
	files      []launcher.ContentFile
	selectable int
	clipboard  string
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
		if file.Directory {
			text.WriteString("[目录] ")
		} else {
			list.selectable++
			if file.Enabled {
				text.WriteString("[启用] ")
			} else {
				text.WriteString("[停用] ")
			}
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
	a.closeModReleaseNotes()
	a.content.savedPacks = nil
	a.content.restoreMods = nil
	a.content.packPreview = nil
	a.content.copyMods = nil
	a.content.comparison = nil
	a.content.listing = nil
	if a.ctx == nil {
		return
	}
	if a.contentCancel != nil {
		a.contentCancel()
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.contentCancel = cancel
	a.contentRequest++
	a.content.mods, a.content.selected, a.content.checking, a.content.checkError = nil, nil, false, ""
	request, kind := a.contentRequest, a.content.kind()
	a.content.reading, a.content.err, a.content.files = true, "", nil
	go func() {
		defer cancel()
		files, err := launcher.ContentInventory(ctx, i, kind)
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
	a.discardStaleModReleaseNotes()
	if p := v.modNotes; p != nil {
		a.modReleaseNotes(c, p)
		return
	}
	if v.id != i.ID || v.revision != i.Revision || v.tab != 0 && !v.kind().Supports(i) {
		*v = contentView{id: i.ID, revision: i.Revision}
		a.refreshContent(i)
	}
	if v.restoreMods != nil {
		a.modRestore(c, i, v.restoreMods)
		return
	}
	if v.packPreview != nil {
		a.resourcePackPreview(c, i, v.packPreview)
		return
	}
	if v.savedPacks != nil {
		a.savedPackOrder(c, i, v.savedPacks)
		return
	}
	if v.copyMods != nil {
		a.modCopy(c, i, v.copyMods)
		return
	}
	if v.comparison != nil {
		a.modComparison(c, i, v.comparison)
		return
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
		if kind == launcher.ResourcePacks && button(c, "游戏上次选择", action.Default, v.reading) {
			a.openSavedPacks(i)
		}
		if kind == launcher.Mods && icon(c, lucide.GitCompareArrows(), "比对实例模组", v.reading) {
			v.comparison = &modComparisonView{}
		}
		if kind == launcher.Mods && len(v.files) > 0 && button(c, "检查更新", action.Default, disabled || v.checking || v.reading) {
			a.checkModUpdates(i)
		}
		if button(c, "添加"+contentLabel(kind)+"…", action.Primary, false) {
			a.openResourceDownloads(i, kind)
		}
		if button(c, "导入…", action.Default, disabled) {
			ext, _, _ := kind.Format(i)
			a.pickPaths(mygo.OpenDialogOptions{Title: "选择本地资源（可多选）", Multiple: true,
				Filters: []mygo.FileFilter{{Name: "游戏资源", Extensions: []string{strings.TrimPrefix(ext, ".")}}}}, func(paths []string) {
				a.importResourceFiles(i, kind, paths)
			})
		}
		if icon(c, lucide.RefreshCw(), "刷新资源", v.reading) {
			a.refreshContent(i)
		}
		if icon(c, lucide.FolderOpen(), "打开"+contentLabel(kind)+"目录", false) {
			a.openPath(filepath.Join(i.GameDir, string(kind)), true)
		}
	})
	kind := v.kind()
	zone := ui.Column(c.Key(fmt.Sprintf("resource-drop-%d-%d-%s", i.ID, i.Revision, kind))).
		Label("本地资源区").Grow(1).MinHeight(0).Gap(8)
	dropOver := false
	if !disabled {
		dropOver = zone.FileDragOver()
		if paths := zone.DroppedFiles(); len(paths) > 0 {
			a.importResourceFiles(i, kind, paths)
		}
	}
	if dropOver {
		ink, wash := brand(c)
		zone.Background(wash).Border(1, ink).Radius(8)
	}
	zone.Children(func() {
		switch {
		case dropOver:
			ui.Column(c).Grow(1).Center().Children(func() {
				display.Empty(c, display.EmptyProps{Icon: lucide.FolderInput(), Title: "松开以导入" + contentLabel(kind), Description: "目标实例 · " + i.Name})
			})
		case v.reading:
			flightLoading(c, "正在读取资源…")
		case v.err != "":
			feedback.Alert(c, v.err, feedback.AlertProps{Type: feedback.Error})
		case len(v.files) == 0:
			ext, _, _ := kind.Format(i)
			hint := "将 " + ext + " 文件拖到此处，或点击「导入」。"
			if kind == launcher.ShaderPacks {
				hint = "将 .zip 文件拖到此处，再到游戏内的光影模组中选择。"
			}
			if a.running != nil {
				hint = "退出游戏后可导入文件。"
			} else if a.busy {
				hint = "当前任务结束后可导入文件。"
			}
			ui.Column(c).Grow(1).Center().Children(func() {
				display.Empty(c, display.EmptyProps{Icon: lucide.PackageOpen(), Title: "暂无本地" + contentLabel(kind), Description: hint})
				a.modRestoreButton(c, i, disabled)
			})
		default:
			a.contentList(c, i, disabled)
		}
	})
}

func (a *desktop) importResourceFiles(i launcher.Instance, kind launcher.ContentKind, paths []string) {
	// Native drop events and file pickers own the original slice.
	sources := slices.Clone(paths)
	a.runInstanceTask(i, fmt.Sprintf("导入 %d 项%s", len(sources), contentLabel(kind)), kind, func(ctx context.Context) error {
		completed, err := launcher.ImportContents(ctx, i, kind, sources)
		if err != nil {
			return fmt.Errorf("已导入 %d / %d 项：%w", completed, len(sources), err)
		}
		return nil
	})
}

func (v *contentView) visibleFiles() []launcher.ContentFile {
	query := strings.ToLower(strings.TrimSpace(v.query))
	if query == "" && v.filter == 0 {
		return v.files
	}
	var files []launcher.ContentFile
	for _, file := range v.files {
		if query != "" && !strings.Contains(strings.ToLower(file.Name+"\n"+file.ModTitle+"\n"+file.ModIDs+"\n"+v.mods[file.DiskName()].Title), query) ||
			v.filter != 0 && file.Directory || v.filter == 1 && !file.Enabled || v.filter == 2 && file.Enabled {
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
		if _, ok := v.selected[file.DiskName()]; ok {
			selected = append(selected, file)
		}
	}
	return selected
}

func (a *desktop) contentList(c *ui.Context, i launcher.Instance, disabled bool) {
	v := &a.content
	if v.kind() == launcher.Mods && (v.checking || v.mods != nil || v.checkError != "") {
		a.modControls(c, i, disabled)
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		oldQuery, oldFilter := v.query, v.filter
		placeholder := "搜索本地文件"
		if v.kind() == launcher.Mods {
			placeholder = "搜索名称、模组 ID 或文件名"
		}
		field.SearchBar(c, &v.query, field.SearchBarProps{Placeholder: placeholder}).Label("搜索本地资源").Grow(1).MinWidth(0)
		action.Segmented(c, &v.filter, []string{"全部资源", "已启用", "已停用"}, action.SegmentedProps{Size: theme.Small}).Label("本地资源筛选")
		if v.kind() == launcher.Mods && button(c, "复制全部模组清单", action.Default, false) {
			c.WriteClipboardThen(installedModReport(v.files), func(err error) {
				if a.page != pageLibrary || a.detailPage != detailContent || a.inspected != i.ID ||
					v.id != i.ID || v.revision != i.Revision || v.kind() != launcher.Mods {
					return
				}
				if err != nil {
					a.message("复制模组清单失败，请重试", err)
					return
				}
				a.message("已复制全部模组清单", nil)
			})
		}
		if oldQuery != v.query || oldFilter != v.filter {
			v.selected, v.scroll = nil, ui.ListState{}
		}
	})
	listing := v.currentListing()
	files := listing.files
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		n := len(v.selected)
		all := n > 0 && n == listing.selectable
		if choice.Checkbox(c, &all, "全选当前列表", choice.CheckboxProps{Indeterminate: n > 0 && n < listing.selectable}).
			Disabled(disabled || listing.selectable == 0).Changed() {
			v.selected = make(map[string]bool, len(files))
			if all {
				for _, file := range files {
					if !file.Directory {
						v.selected[file.DiskName()] = file.Enabled
					}
				}
			}
		}
		muted(c, fmt.Sprintf("%d / %d 项", len(files), len(v.files))).FontSize(12).Grow(1)
		a.modRestoreButton(c, i, disabled)
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
		return files[row].DiskName()
	}).Grow(1).MinHeight(0).Rows(func(row ui.ListRow) {
		c, file := row.Context, files[row.Index]
		if row.Index > 0 {
			layout.Divider(c, layout.DividerProps{})
		}
		key := file.DiskName()
		ui.Row(c).MinHeight(46).Padding(0, 12).Gap(8).AlignItems(ui.Center).Children(func() {
			_, checked := v.selected[key]
			if choice.Checkbox(c, &checked, "", choice.CheckboxProps{}).Label("选择资源 " + file.Name).
				Disabled(disabled || file.Directory).Changed() {
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
			a.contentFileTitle(c, i, file)
			if v.kind() == launcher.ResourcePacks && action.Button(c, "预览", action.ButtonProps{Variant: action.Default}).Label("预览 "+key).Clicked() {
				a.openPackPreview(i, file)
			}
			if file.Directory {
				muted(c, "目录").FontSize(12)
				return
			}
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
				a.runInstanceTask(i, label+contentLabel(kind), kind, func(context.Context) error {
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
			if v.kind() == launcher.Mods {
				if button(c, "复制到实例…", action.Default, disabled) {
					v.copyMods = &modCopyView{selected: v.selection(files)}
				}
				if button(c, "备份 ZIP…", action.Default, disabled) {
					a.chooseModBackup(i, v.selection(files))
				}
			}
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
	label := "批量停用"
	if enabled {
		label = "批量启用"
	}
	a.runInstanceTask(i, label+contentLabel(kind), kind, func(ctx context.Context) error {
		changed, err := launcher.SetContentEnabledBatch(ctx, i, kind, files, enabled)
		if err != nil {
			return fmt.Errorf("已更改 %d 项，其余资源请刷新后检查：%w", changed, err)
		}
		return nil
	})
}

func (a *desktop) runInstanceTask(i launcher.Instance, label string, kind launcher.ContentKind, work func(context.Context) error) {
	var resource *resourceTask
	if kind != "" {
		resource = &resourceTask{Kind: string(kind)}
	}
	a.runContentTask(i, label, resource, func(ctx context.Context, _ func(launcher.Progress)) error { return work(ctx) })
}

func (a *desktop) runContentTask(i launcher.Instance, label string, resource *resourceTask, work func(context.Context, func(launcher.Progress)) error) {
	a.runContentTaskWithResult(i, label, resource, work, nil)
}

func (a *desktop) runContentTaskWithResult(i launcher.Instance, label string, resource *resourceTask, work func(context.Context, func(launcher.Progress)) error, result func(*taskRecord)) {
	current, ok := a.state.Find(i.ID)
	if !ok || current.Revision != i.Revision || a.busy || a.running != nil || a.ctx == nil {
		return
	}
	row := taskRecord{InstanceID: i.ID, Revision: i.Revision, Name: i.Name, Version: i.Version, Action: label, At: time.Now(), Resource: resource}
	a.runFileTaskWithResult(row, work, result, func(error) {
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

func (a *desktop) runFileTask(row taskRecord, work func(context.Context, func(launcher.Progress)) error, refresh func(error)) {
	a.runFileTaskWithResult(row, work, nil, refresh)
}

func (a *desktop) runFileTaskWithResult(row taskRecord, work func(context.Context, func(launcher.Progress)) error, result func(*taskRecord), refresh func(error)) {
	queuedCurseMod := row.ID != "" && row.Resource != nil && row.Resource.Source == launcher.CurseForgeMods
	row = row.Submitted()
	ctx, cancel := context.WithCancel(a.ctx)
	a.busy, a.taskName, a.cancelTask = true, row.Name, cancel
	a.taskCanceling = false
	a.task, a.notice = launcher.Progress{Stage: row.Action}, ""
	a.writes.Go(func() {
		defer cancel()
		err := tasks.Run(ctx, work, func(p launcher.Progress) { a.post(func() { a.task = p }) })
		row.Status = tasks.Outcome(err)
		if queuedCurseMod && errors.Is(err, launcher.ErrCurseForgeAccessDenied) {
			row.Status = tasks.WaitingForKey
		}
		if err != nil {
			row.Detail = err.Error()
			if row.Pack.Archive != "" {
				row.Pack.BackupDir = launcher.RollbackDirectory(err)
			}
		} else {
			row.Pack = tasks.PackUpdate{}
			if result != nil {
				result(&row)
			}
		}
		noticeErr := err
		if plainTaskCancellation(row) {
			row.Detail, noticeErr = "", nil
		}
		a.saveTask(row, func() {
			a.busy, a.cancelTask, a.task = false, nil, launcher.Progress{}
			a.taskCanceling = false
			a.message(row.Action+" · "+row.Status, noticeErr)
			if refresh != nil {
				refresh(err)
			}
		})
	})
}

func fileSizeLabel[T int64 | uint64](bytes T) string {
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

func (a *desktop) contentFileTitle(c *ui.Context, i launcher.Instance, f launcher.ContentFile) {
	v := &a.content
	key := f.DiskName()
	m, found := v.mods[key]
	if (!found || m.Current.ID == "") && f.ModTitle == "" {
		display.Text(c, f.Name, display.TextProps{Strong: true, Rows: 1}).Grow(1).MinWidth(0).Ellipsis("…")
		return
	}
	updateKey := m.UpdateKey()
	skipped := a.state.SkippedModUpdates[i.ID][updateKey]
	title, detail := f.ModTitle, f.Name
	version, missing := m.Current, m.Unconfirmed
	if m.Current.ID != "" {
		title = m.Title
		if title == "" {
			title = f.Name
		}
		if m.Current.Number != "" {
			title += " · " + m.Current.Number
		}
		detail = "Modrinth · " + detail
		if updateKey != "" {
			number := cmp.Or(m.Update.Number, m.Update.ID)
			if skipped {
				detail = "已跳过 " + number + " · " + detail
				missing = m.CurrentUnconfirmed
			} else {
				title += " → " + number
				version = m.Update
			}
		}
	}
	ui.Column(c).Grow(1).MinWidth(0).Children(func() {
		display.Text(c, title, display.TextProps{Strong: true, Rows: 1}).Ellipsis("…")
		muted(c, detail).FontSize(11).SingleLine().Ellipsis("…")
	})
	if updateKey != "" {
		if icon(c.Key("update-notes"), lucide.FileText(), "查看发布说明 "+key, v.checking) {
			a.openModReleaseNotes(c, i, key, m)
		}
		label := "跳过此版本"
		if skipped {
			label = "恢复提示"
		}
		if button(c.Key(updateKey), label, action.Text, a.busy || v.checking) {
			a.setModUpdateSkipped(i, updateKey, !skipped)
		}
	}
	if len(missing) > 0 && button(c, fmt.Sprintf("核对前置 (%d)", len(missing)), action.Default, false) {
		a.openResourceDownloads(i, launcher.Mods)
		p := a.browserFor(launcher.Mods)
		if p.source != "" {
			p.source = ""
			p.resetCatalog()
		}
		p.abort()
		p.clearProject()
		p.searched, p.searchBusy, p.err = true, false, ""
		p.packSelection = packSelection{project: launcher.PackProject{ID: version.ProjectID, Title: title}, versions: []launcher.PackVersion{version}, version: version.ID}
	}
}

func (a *desktop) setModUpdateSkipped(i launcher.Instance, candidate string, skip bool) {
	next := a.state.Clone()
	if err := next.SetModUpdateSkipped(i.ID, i.Revision, candidate, skip); err != nil {
		a.message("无法保存更新提示", err)
		return
	}
	a.save(next, 0, func(err error) {
		if err != nil {
			a.message("无法保存更新提示", err)
		}
	})
}

func (a *desktop) checkModUpdates(i launcher.Instance) {
	v := &a.content
	req, g, ctx := a.contentRequest, a.game, a.ctx
	v.checking, v.checkError = true, ""
	go func() {
		mods, err := g.CheckMods(ctx, i)
		a.post(func() {
			if a.contentRequest != req {
				return
			}
			v.checking = false
			if err != nil {
				v.checkError = err.Error()
				return
			}
			v.mods, v.listing = mods, nil
		})
	}()
}

func (a *desktop) modControls(c *ui.Context, i launcher.Instance, disabled bool) {
	v := &a.content
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		switch {
		case v.checking:
			muted(c, "正在识别模组并检查兼容版本…")
		case v.mods != nil:
			var selected []launcher.InstalledMod
			known, updates := 0, 0
			for _, f := range v.files {
				key := f.DiskName()
				m := v.mods[key]
				if m.Current.ID != "" {
					known++
				}
				if candidate := m.UpdateKey(); candidate != "" && !a.state.SkippedModUpdates[i.ID][candidate] {
					updates++
					if _, checked := v.selected[key]; checked {
						selected = append(selected, m)
					}
				}
			}
			muted(c, fmt.Sprintf("已识别 %d/%d · 可更新 %d", known, len(v.files), updates))
			if updates > 0 && button(c, fmt.Sprintf("更新选中 (%d)", len(selected)), action.Primary, len(selected) == 0 || disabled) {
				game := a.game
				a.runContentTask(i, "更新模组", nil, func(ctx context.Context, report func(launcher.Progress)) error {
					var failures []error
					for n, m := range selected {
						if err := ctx.Err(); err != nil {
							return errors.Join(append(failures, err)...)
						}
						report(launcher.Progress{Stage: "更新 " + m.DiskName(), Done: n, Total: len(selected)})
						if err := game.UpdateMod(ctx, i, m); err != nil {
							failures = append(failures, fmt.Errorf("%s: %w", m.DiskName(), err))
						}
					}
					return errors.Join(failures...)
				})
			}
		}
	})
	if v.checkError != "" {
		feedback.Alert(c, v.checkError, feedback.AlertProps{Type: feedback.Error})
	}
}

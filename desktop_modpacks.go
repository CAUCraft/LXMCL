package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
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

const (
	modpackModrinth = iota
	modpackCurseForge
	modpackLocal
)

type modpackView struct {
	mode                                  int
	key, keyDraft                         string
	keyOpen                               bool
	query, applied, game                  string
	searched, searching                   bool
	results                               launcher.ModpackResults
	page                                  int
	searchErr                             string
	project                               launcher.ModpackProject
	releases                              []launcher.ModpackRelease
	releasePage                           int
	releaseMore, listing                  bool
	releaseID, releaseErr                 string
	preparing                             bool
	archive, name, prepareErr, installErr string
	prepared                              launcher.PreparedModpack
	request                               uint64
	cancel                                context.CancelFunc
	projectsScroll, releasesScroll        ui.ScrollState
}

func (p *modpackView) source() launcher.ModpackSource {
	if p.mode == modpackCurseForge {
		return launcher.CurseForgeModpacks
	}
	return launcher.ModrinthModpacks
}

func (p *modpackView) stop() {
	p.request++
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.searching, p.listing, p.preparing = false, false, false
}

func (p *modpackView) begin(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc, uint64) {
	p.stop()
	ctx, cancel := context.WithTimeout(parent, timeout)
	p.cancel = cancel
	return ctx, cancel, p.request
}

func (p *modpackView) clearPreview() {
	p.prepared = launcher.PreparedModpack{}
	p.archive, p.name, p.prepareErr, p.installErr = "", "", "", ""
}

func (a *desktop) searchModpackProjects(page int) {
	p := &a.modpacks
	if a.ctx == nil || p.mode == modpackLocal || (p.mode == modpackCurseForge && p.key == "") {
		return
	}
	ctx, cancel, request := p.begin(a.ctx, 25*time.Second)
	p.searched, p.searching, p.page, p.searchErr = true, true, page, ""
	p.results, p.project, p.projectsScroll = launcher.ModpackResults{}, launcher.ModpackProject{}, ui.ScrollState{}
	p.clearPreview()
	source, key, query, game := p.source(), p.key, p.applied, p.game
	go func() {
		defer cancel()
		result, err := launcher.SearchModpacks(ctx, source, key, query, game, page)
		a.post(func() {
			if p.request != request {
				return
			}
			p.searching, p.results = false, result
			if err != nil {
				p.searchErr = err.Error()
			}
		})
	}()
}

func (a *desktop) listModpackReleases(project launcher.ModpackProject, page int) {
	p := &a.modpacks
	if a.ctx == nil {
		return
	}
	ctx, cancel, request := p.begin(a.ctx, 25*time.Second)
	p.clearPreview()
	p.project, p.releasePage, p.listing, p.releaseMore = project, page, true, false
	p.releases, p.releaseID, p.releaseErr, p.releasesScroll = nil, "", "", ui.ScrollState{}
	game, key := p.game, p.key
	go func() {
		defer cancel()
		releases, more, err := launcher.ListModpackReleases(ctx, project, key, game, page)
		a.post(func() {
			if p.request != request {
				return
			}
			p.listing, p.releases, p.releaseMore = false, releases, more
			if err != nil {
				p.releaseErr = err.Error()
			}
		})
	}()
}

func (a *desktop) readModpack(filename string, release launcher.ModpackRelease) {
	p := &a.modpacks
	if a.ctx == nil || a.busy {
		return
	}
	ctx, cancel, request := p.begin(a.ctx, 6*time.Minute)
	p.clearPreview()
	p.preparing, p.archive = true, filepath.Base(filename)
	if filename == "" {
		p.archive = release.Filename
	}
	game := a.game
	go func() {
		defer cancel()
		var pack launcher.PreparedModpack
		var err error
		if filename != "" {
			pack, err = launcher.PrepareLocalModpack(ctx, filename)
		} else {
			pack, err = game.PrepareOnlineModpack(ctx, release)
		}
		a.post(func() {
			if p.request != request {
				return
			}
			p.preparing = false
			if err != nil {
				p.prepareErr = err.Error()
				return
			}
			p.prepared = pack
			name := strings.TrimSpace(pack.Info().Name)
			if name == "" {
				name = strings.TrimSuffix(p.archive, filepath.Ext(p.archive))
			}
			p.name = modpackInstanceName(a.state, name)
		})
	}()
}

func modpackInstanceName(state launcher.State, title string) string {
	title = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return ' '
		}
		return r
	}, title))
	if title == "" {
		title = "整合包"
	}
	runes := []rune(title)
	for n := 1; ; n++ {
		suffix := ""
		if n > 1 {
			suffix = fmt.Sprintf(" (%d)", n)
		}
		name := strings.TrimSpace(string(runes[:min(len(runes), 48-len(suffix))])) + suffix
		if !slices.ContainsFunc(state.Instances, func(i launcher.Instance) bool { return strings.EqualFold(i.Name, name) }) {
			return name
		}
	}
}

func (a *desktop) pickModpack() {
	a.pickPath(mygo.OpenDialogOptions{Title: "选择 Minecraft 整合包", ButtonLabel: "选择整合包",
		Filters: []mygo.FileFilter{
			{Name: "Minecraft 整合包", Extensions: []string{"mrpack", "zip"}},
			{Name: "Modrinth 整合包", Extensions: []string{"mrpack"}},
			{Name: "CurseForge 整合包", Extensions: []string{"zip"}},
		}}, func(path string) {
		if a.modpacks.mode == modpackLocal {
			a.readModpack(path, launcher.ModpackRelease{})
		}
	})
}

func (a *desktop) modpackPage(c *ui.Context) {
	p := &a.modpacks
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		before := p.mode
		action.Segmented(c, &p.mode, []string{"Modrinth", "CurseForge", "本地导入"},
			action.SegmentedProps{Size: theme.Small}).Label("整合包来源")
		if p.mode != before {
			p.stop()
			p.clearPreview()
			p.project, p.searched, p.searchErr = launcher.ModpackProject{}, false, ""
			c.Invalidate()
		}
		ui.Spacer(c).Grow(1)
		if p.mode == modpackCurseForge && button(c, "API Key…", action.Default, false) {
			p.keyOpen = true
		}
	})
	if p.mode != modpackLocal && (p.mode != modpackCurseForge || p.key != "") {
		if len(a.releases) == 0 && a.catalogErr == "" {
			a.refreshCatalog()
		}
		if !p.searched {
			p.applied = strings.TrimSpace(p.query)
			a.searchModpackProjects(0)
		}
		a.modpackSearchBar(c, p)
	}
	w, _ := c.Size()
	splitWorkspace(c, catalogListWidth(w), func() {
		switch {
		case p.mode == modpackLocal:
			if button(c, "选择整合包文件…", action.Default, a.busy || a.pickerBusy || p.preparing) {
				a.pickModpack()
			}
			muted(c, "Modrinth .mrpack 或 CurseForge .zip").FontSize(12)
		case p.mode == modpackCurseForge && p.key == "":
			muted(c, "CurseForge").FontSize(13)
		default:
			a.modpackResults(c, p)
		}
	}, func() {
		right := c.Key("modpack-detail")
		switch {
		case p.preparing:
			heading(right, p.archive)
			flightLoading(right, "正在读取整合包…")
			if button(right, "取消读取", action.Default, false) {
				p.stop()
				p.clearPreview()
			}
		case p.prepared.Info().Source != "":
			a.modpackConfirmation(right, p)
		case p.mode == modpackLocal:
			if p.prepareErr != "" {
				feedback.Alert(right, p.prepareErr, feedback.AlertProps{Type: feedback.Error})
			} else {
				ui.Column(right).Grow(1).Center().Children(func() { muted(right, "选择文件以查看整合包内容") })
			}
		case p.mode == modpackCurseForge && p.key == "":
			ui.Column(right).Grow(1).Center().Children(func() {
				display.Empty(right, display.EmptyProps{Icon: lucide.KeyRound(), Title: "需要 CurseForge API Key",
					Description: "设置 API Key 后可查询和下载 CurseForge 整合包。", Action: func() {
						if button(right, "设置 API Key…", action.Primary, false) {
							p.keyOpen = true
						}
					}})
			})
		default:
			a.modpackVersions(right, p)
		}
	})
}

func (a *desktop) modpackSearchBar(c *ui.Context, p *modpackView) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		submit := false
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			submit = field.Input(c, &p.query, field.InputProps{Placeholder: "搜索整合包", Prefix: lucide.Search()}).
				Label("搜索整合包").Submitted()
		})
		options := []field.SelectOption{{Value: "", Label: "所有游戏版本"}}
		for _, v := range a.releases {
			options = append(options, field.SelectOption{Value: v.ID, Label: v.ID})
		}
		before := p.game
		catalogPicker(c.Key("modpack-game-version"), &p.game, options, "整合包游戏版本筛选", "", false).Width(140).Shrink(0)
		if button(c, "搜索", action.Default, false) || submit || p.game != before {
			p.applied = strings.TrimSpace(p.query)
			a.searchModpackProjects(0)
		}
	})
}

func (a *desktop) modpackResults(c *ui.Context, p *modpackView) {
	switch {
	case p.searching:
		flightLoading(c, "正在搜索…")
		return
	case p.searchErr != "":
		feedback.Alert(c, p.searchErr, feedback.AlertProps{Type: feedback.Error})
		if button(c, "重试搜索", action.Default, false) {
			a.searchModpackProjects(p.page)
		}
		return
	}
	items := make([]display.ListItem, 0, len(p.results.Projects))
	for _, m := range p.results.Projects {
		items = append(items, display.ListItem{Key: m.ID, Label: m.Name, Description: m.Author,
			Avatar: func() { a.projectIcon(c, "modpack-"+m.ID, m.IconURL, lucide.Archive(), 28) }})
	}
	layout.ScrollArea(c.Key("modpack-projects"), layout.ScrollAreaProps{State: &p.projectsScroll}, func() {
		if len(items) == 0 {
			muted(c, "本页没有整合包").Padding(6)
		}
		if id := display.List(c, display.ListProps{Items: items, Active: p.project.ID, Compact: true}); id != "" && id != p.project.ID {
			if n := slices.IndexFunc(p.results.Projects, func(m launcher.ModpackProject) bool { return m.ID == id }); n >= 0 {
				a.listModpackReleases(p.results.Projects[n], 0)
				c.Invalidate()
			}
		}
	}).Grow(1).MinHeight(0)
	layout.Divider(c, layout.DividerProps{})
	modpackPager(c, p.page, (p.page+1)*launcher.PackPageSize < p.results.Total, a.searchModpackProjects)
}

func modpackPager(c *ui.Context, page int, more bool, change func(int)) {
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		if icon(c, lucide.ChevronLeft(), "上一页", page == 0) {
			change(page - 1)
		}
		muted(c, fmt.Sprintf("第 %d 页", page+1)).FontSize(12).Grow(1)
		if icon(c, lucide.ChevronRight(), "下一页", !more || page+1 >= 10000/launcher.PackPageSize) {
			change(page + 1)
		}
	})
}

func (a *desktop) modpackVersions(c *ui.Context, p *modpackView) {
	if p.project.ID == "" {
		ui.Column(c).Grow(1).Center().Children(func() { muted(c, "选择一个整合包查看版本") })
		return
	}
	a.projectHeading(c, "modpack-heading", p.project.Name, p.project.IconURL, lucide.Archive(), func() {
		if p.project.URL != "" && icon(c, lucide.ExternalLink(), "打开整合包项目页面", false) {
			c.OpenURL(p.project.URL)
		}
	})
	if p.project.Summary != "" {
		display.Text(c, p.project.Summary, display.TextProps{Type: display.TextSecondary, FontSize: 13, Rows: 2}).Ellipsis("…")
	}
	if p.listing {
		flightLoading(c, "正在读取整合包版本…")
		return
	}
	if p.releaseErr != "" {
		feedback.Alert(c, p.releaseErr, feedback.AlertProps{Type: feedback.Error})
		if button(c, "重试读取版本", action.Default, false) {
			a.listModpackReleases(p.project, p.releasePage)
		}
		return
	}
	if len(p.releases) == 0 {
		muted(c, "本页没有可选的客户端版本")
	}
	items := make([]display.ListItem, 0, len(p.releases))
	var selected launcher.ModpackRelease
	for _, r := range p.releases {
		title := r.Title
		if title == "" {
			title = r.Filename
		}
		games := strings.Join(r.Games[:min(3, len(r.Games))], " / ")
		if len(r.Games) > 3 {
			games += " …"
		}
		detail := strings.Trim(strings.Join([]string{games, strings.Join(r.Loaders, " / ")}, " · "), " ·")
		if r.Unavailable != "" {
			detail = r.Unavailable
		}
		channel := map[string]string{"release": "正式版", "beta": "Beta", "alpha": "Alpha"}[r.Channel]
		items = append(items, display.ListItem{Key: r.ID, Label: title, Description: detail, Icon: lucide.FileArchive(), Extra: channel})
	}
	layout.ScrollArea(c.Key("modpack-versions"), layout.ScrollAreaProps{State: &p.releasesScroll}, func() {
		if id := display.List(c, display.ListProps{Items: items, Active: p.releaseID, Compact: true}); id != "" && id != p.releaseID {
			p.releaseID, p.prepareErr = id, ""
			c.Invalidate()
		}
	}).Grow(1).MinHeight(0)
	for _, r := range p.releases {
		if p.releaseID == r.ID {
			selected = r
			break
		}
	}
	if p.prepareErr != "" {
		feedback.Alert(c, p.prepareErr, feedback.AlertProps{Type: feedback.Error})
	}
	layout.Divider(c, layout.DividerProps{})
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		if p.releasePage > 0 || p.releaseMore {
			modpackPager(c.Key("modpack-version-page"), p.releasePage, p.releaseMore, func(page int) {
				a.listModpackReleases(p.project, page)
			})
		}
		ui.Spacer(c).Grow(1)
		if button(c, "保存到文件夹…", action.Default, a.busy || a.running != nil || a.pickerBusy || !selected.Downloadable()) {
			release, title, game := selected, p.project.Name, a.game
			a.pickDownloadFolder(func(folder string) {
				a.runFolderDownload(folder, "保存整合包", title, nil, func(ctx context.Context, report func(launcher.Progress)) error {
					return game.DownloadModpackFile(ctx, release, folder, report)
				})
			})
		}
		if button(c, "安装为实例…", action.Primary, a.busy || selected.ID == "" || selected.Unavailable != "") {
			a.readModpack("", selected)
		}
	})
}

func (a *desktop) modpackConfirmation(c *ui.Context, p *modpackView) {
	info := p.prepared.Info()
	name := info.Name
	if name == "" {
		name = p.archive
	}
	heading(c, name, func() {
		if p.mode != modpackLocal && button(c, "返回版本列表", action.Text, false) {
			p.clearPreview()
			c.Invalidate()
		}
	})
	if p.prepared.Info().Source == "" {
		return
	}
	loader := (launcher.Instance{Loader: info.Loader, LoaderVersion: info.LoaderVersion}).LoaderName()
	subtitle := string(info.Source)
	if info.Version != "" {
		subtitle += " · " + info.Version
	}
	muted(c, subtitle).FontSize(13).SingleLine().Ellipsis("…")
	needsKey := info.Source == launcher.CurseForgeModpacks && info.Downloads > 0 && p.key == ""
	layout.Divider(c, layout.DividerProps{})
	layout.ScrollArea(c.Key("modpack-install-config"), layout.ScrollAreaProps{}, func() {
		ui.Column(c).Gap(8).Children(func() {
			muted(c, fmt.Sprintf("Minecraft %s · %s", info.Minecraft, loader))
			muted(c, fmt.Sprintf("%d 个必需依赖 · %d 个内含文件", info.Downloads, info.Overrides)).FontSize(13)
			if info.Loader == launcher.LoaderForge || info.Loader == launcher.LoaderNeoForge {
				muted(c, "首次安装加载器需要兼容的 Java。").FontSize(12)
			}
			muted(c, "实例名称").FontSize(12)
			field.Input(c.Key("modpack-instance-name"), &p.name, field.InputProps{Placeholder: "输入新实例名称"}).
				Label("整合包实例名称").Disabled(a.busy)
			if needsKey {
				muted(c, "下载 CurseForge 依赖需要 API Key。").FontSize(12)
				if button(c, "设置 API Key…", action.Default, false) {
					p.keyOpen = true
				}
			}
			if p.installErr != "" {
				feedback.Alert(c, p.installErr, feedback.AlertProps{Type: feedback.Error})
			}
		})
	}).Grow(1).MinHeight(0)
	layout.Divider(c, layout.DividerProps{})
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		muted(c, "安装到新实例，使用独立游戏目录").FontSize(12).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
		if button(c, "安装整合包", action.Primary, a.busy || a.running != nil || needsKey || strings.TrimSpace(p.name) == "") {
			a.installModpack()
		}
	})
}

func (a *desktop) curseKeyDialog(c *ui.Context) {
	p := &a.modpacks
	if !p.keyOpen {
		p.keyDraft = ""
		return
	}
	feedback.Modal(c, &p.keyOpen, feedback.ModalProps{Title: "CurseForge API Key", Width: 420, NoFooter: true}, func() {
		ui.Column(c).Gap(10).Children(func() {
			muted(c, "填写 CurseForge 开发者控制台提供的 API Key。仅在本次启动器运行期间保留。").FontSize(13)
			submit := field.InputPassword(c, &p.keyDraft, field.InputProps{Placeholder: "输入 API Key"}).Label("CurseForge API Key").Submitted()
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				if button(c, "打开控制台", action.Text, false) {
					c.OpenURL("https://console.curseforge.com/")
				}
				ui.Spacer(c).Grow(1)
				if button(c, "取消", action.Default, false) {
					p.keyOpen = false
				}
				key := strings.TrimSpace(p.keyDraft)
				if (button(c, "应用", action.Primary, key == "") || submit) && key != "" {
					p.key, p.keyOpen, p.keyDraft = key, false, ""
					if p.mode == modpackCurseForge && p.prepared.Info().Source == "" {
						p.applied = strings.TrimSpace(p.query)
						a.searchModpackProjects(0)
					}
				}
			})
		})
	})
}

func (a *desktop) installModpack() {
	p := &a.modpacks
	pack, key := p.prepared, p.key
	info := pack.Info()
	if a.ctx == nil || !a.ready || a.busy || a.running != nil || info.Source == "" ||
		(info.Source == launcher.CurseForgeModpacks && info.Downloads > 0 && key == "") {
		return
	}
	next, i, err := newLoaderInstallation(a.state, p.name, info.Minecraft, info.Loader, info.LoaderVersion)
	if err != nil {
		p.installErr = err.Error()
		return
	}
	p.installErr = ""
	ctx, cancel := context.WithCancel(a.ctx)
	a.busy, a.taskName, a.cancelTask = true, i.Name, cancel
	a.task, a.notice = launcher.Progress{Stage: "检查整合包清单"}, ""
	game, store := a.game, a.store
	record := (taskRecord{Name: i.Name, Version: i.Version, Action: "安装整合包", At: time.Now()}).Submitted()
	a.writes.Go(func() {
		defer cancel()
		var last time.Time
		var stage string
		err := game.InstallModpack(ctx, store, next, i, pack, key, func(progress launcher.Progress) {
			if progress.Stage != stage || progress.Done == progress.Total || time.Since(last) > 140*time.Millisecond {
				last, stage = time.Now(), progress.Stage
				a.post(func() { a.task = progress })
			}
		})
		switch {
		case errors.Is(err, context.Canceled):
			record.Status, record.Detail = taskCanceled, "已校验的文件可在下次安装时复用"
		case err != nil:
			record.Status, record.Detail = taskFailed, err.Error()
		default:
			record.InstanceID, record.Revision = i.ID, i.Revision
			record.Status, record.Detail = taskCompleted, string(info.Source)+" · "+info.Version
		}
		a.saveTask(record, func() {
			a.busy, a.cancelTask, a.task = false, nil, launcher.Progress{}
			if err == nil {
				a.state = next
				if a.installed == nil {
					a.installed = make(map[int]bool)
				}
				a.installed[i.ID] = true
				if p.prepared == pack {
					p.clearPreview()
					if a.page == pageDownloads && a.downloadPage == downloadModpacks {
						a.inspect(i)
					}
				}
				a.message("整合包已安装 · "+i.Name, nil)
			} else if record.Status == taskCanceled {
				a.message("整合包安装已取消", nil)
			} else {
				if p.prepared == pack {
					p.installErr = err.Error()
				}
				a.message("整合包安装失败 · "+i.Name, err)
				a.noticeJava = errors.Is(err, launcher.ErrJavaRequired)
				a.noticeTask = true
			}
		})
	})
}

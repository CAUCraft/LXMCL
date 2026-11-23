package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/CAUCraft/LXMCL/internal/playtime"
	"github.com/CAUCraft/LXMCL/internal/saves"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *desktop) post(fn func()) {
	if a.ctx.Err() != nil {
		return
	}
	mygo.RunOnMain(func() {
		if a.ctx.Err() == nil {
			fn()
			a.window.Invalidate()
		}
	})
}

func (a *desktop) load() {
	if a.ctx == nil || a.loading || a.ready {
		return
	}
	a.loading, a.loadErr = true, ""
	go func() {
		s, err := a.store.Load()
		if err != nil {
			a.post(func() { a.loading, a.loadErr = false, err.Error() })
			return
		}
		icons := loadInstanceIcons(s)
		played, playErr := playtime.Load(a.store.Root)
		account, accountErr := a.store.LoadMicrosoftAccount()
		history, historyErr := a.journal.Load(a.store.Root)
		pending, queueErr := readDownloadQueue(a.store.Root, history)
		curseForgeKey := launcher.LoadCurseForgeAPIKey(a.store.Root)
		installed := readInstallStatus(a.game, s.Instances, "")
		a.post(func() {
			a.loading = false
			a.state, a.draft, a.installed, a.ready = s, s.Settings, installed, true
			a.playtimes, a.playtimeErr = played, playErr
			a.instanceIcons = icons
			a.libraryGroup = s.LibraryGroup
			a.modpacks.key = curseForgeKey
			a.memory = strconv.Itoa(s.Settings.Memory)
			a.restoreTaskHistory(history)
			a.queue.pending, a.queue.loadError = pending, queueErr
			if queueErr != nil {
				a.message("读取下载队列失败", queueErr)
			}
			a.microsoftAccount = account
			if historyErr != nil {
				a.historyReadErr = historyErr.Error()
				a.message("无法读取任务记录", historyErr)
				a.noticeHistory = true
			}
			if accountErr != nil {
				a.message("无法读取 Microsoft 登录", accountErr)
				a.noticeAccount = true
			}
			if s.MicrosoftProfile != "" && s.MicrosoftProfile == account.Profile.ID {
				a.startAppearance()
			}
			if p, ok := s.Station(s.StationKey); ok {
				a.loadStationPreview(p, false)
			}
		})
	}()
}

func (a *desktop) message(s string, err error) {
	a.notice, a.noticeError = s, err != nil
	a.noticeJava, a.noticeAccount, a.noticeHistory = false, false, false
	a.noticeTaskKey = ""
	a.noticeJavaTarget = launcher.Instance{}
	if err != nil {
		a.notice = fmt.Sprintf("%s：%s", s, err)
	}
}

func (a *desktop) save(next launcher.State, checkID int, done func(error)) {
	if !a.ready || a.busy {
		return
	}
	a.busy = true
	i, found := next.Find(checkID)
	check := checkID != 0 && found
	_, registered := a.state.Find(checkID)
	save := a.store.Save
	switch {
	case check && !registered:
		save = func(state launcher.State) error { return a.store.RegisterInstance(state, i) }
	case next.Settings.GameRoot != a.state.Settings.GameRoot:
		save = a.store.SaveWithNewGameRoot
	}
	a.writes.Go(func() {
		err := save(next)
		installed := err == nil && check && a.game.Installed(i)
		a.post(func() {
			a.busy = false
			if err == nil {
				a.state = next
				if check {
					a.installed[i.ID] = installed
				}
			}
			done(err)
		})
	})
}

func (a *desktop) choose(id int) {
	if _, ok := a.state.Find(id); !ok || id == a.state.Selected {
		return
	}
	next := a.state.Clone()
	next.Selected = id
	a.save(next, 0, func(err error) {
		if err != nil {
			a.message("无法保存启动实例", err)
		}
	})
}

func (a *desktop) create() {
	a.openInstall("", false)
}

func (a *desktop) edit(i launcher.Instance) {
	a.form = editor{open: true, mode: "rename", target: i, name: i.Name, focus: true}
}

func (a *desktop) changeVersion(i launcher.Instance) {
	a.form = editor{open: true, mode: "version", target: i, version: i.Version}
	if i.UsesInstaller() || i.Quilt() {
		a.form.loaders = currentLoaderSelection(i)
	}
	if len(a.releases) == 0 {
		a.refreshCatalog()
	}
}

func (a *desktop) duplicate(i launcher.Instance) {
	name := []rune(i.Name)
	title := string(name[:min(len(name), 45)]) + " 副本"
	a.form = editor{open: true, mode: "duplicate", target: i, focus: true,
		name: availableInstanceName(a.state, title, "副本")}
}

func currentLoaderSelection(i launcher.Instance) *loaderSelection {
	p := &loaderSelection{version: i.Version}
	for n, kind := range installLoaders {
		if strings.EqualFold(i.Loader, kind) {
			p.loader = n
			break
		}
	}
	return p
}

func (a *desktop) configureLoader(i launcher.Instance) {
	a.form = editor{open: true, mode: "loader", target: i, loaders: currentLoaderSelection(i)}
}

func (a *desktop) inspect(i launcher.Instance) {
	if !matchesInstanceSearch(i, strings.ToLower(strings.TrimSpace(a.query))) {
		a.query, a.scroll.Y = "", 0
	}
	if !a.matchesCacheFilter(i) {
		a.instanceFilter, a.scroll.Y = 0, 0
	}
	if !a.matchesGroup(i.ID) {
		a.libraryGroup, a.scroll.Y = nil, 0
	}
	a.page, a.inspected = pageLibrary, i.ID
	a.detailPage = detailOverview
}

func (a *desktop) submitEditor() {
	if a.busy {
		return
	}
	form := a.form
	next := a.state.Clone()
	var err error
	checkID := 0
	switch form.mode {
	case "icon":
		if form.icon == nil || form.icon.reading || a.pickerBusy {
			return
		}
		err = next.SetInstanceIcon(form.target.ID, form.target.Revision, form.icon.original, form.icon.encoded)
	case "renameGroup":
		err = next.RenameInstanceGroup(form.originalGroup, form.name)
	case "group":
		err = next.SetInstanceGroup(form.target.ID, form.target.Revision, form.originalGroup, form.name)
	case "note":
		err = next.SetInstanceNote(form.target.ID, form.target.Revision, form.originalNote, form.note)
	case "rename":
		err = next.Rename(form.target.ID, form.target.Revision, form.name)
	case "export":
		if strings.TrimSpace(form.version) == "" {
			a.form.err = "请输入整合包版本"
			return
		}
		a.pickDownloadFolder(func(folder string) {
			a.form.open = false
			a.runInstanceTask(form.target, "导出整合包", "", func(ctx context.Context) error {
				_, err := launcher.ExportInstance(ctx, form.target, folder, form.version, form.files)
				return err
			})
		})
		return
	case "duplicate":
		var duplicate launcher.Instance
		duplicate, err = next.Duplicate(form.target.ID, form.target.Revision, form.name, form.copyGroup)
		checkID = duplicate.ID
		if err == nil && form.files != (launcher.PortableFiles{}) {
			if a.running != nil {
				a.form.err = "请先退出游戏再复制文件"
				return
			}
			a.copyInstanceContent(form, next, duplicate)
			return
		}
	case "loader":
		if a.runningID == form.target.ID {
			a.form.err = "请先退出此实例的游戏"
			return
		}
		p := form.loaders
		if p == nil || !p.validLoader() {
			a.form.err = "请从此游戏版本的目录中选择加载器版本"
			return
		}
		err = next.ConfigureModLoader(form.target.ID, form.target.Revision, p.kind(), p.loaderVersion)
		checkID = form.target.ID
	case "version":
		if a.runningID == form.target.ID {
			a.form.err = "请先退出此实例的游戏"
			return
		}
		version := strings.TrimSpace(form.version)
		loaderVersion := form.target.LoaderVersion
		if p := form.loaders; p != nil && version != form.target.Version {
			if p.version != version || !p.validLoader() {
				a.form.err = "请选择与目标游戏版本兼容的加载器版本"
				return
			}
			loaderVersion = p.loaderVersion
		}
		err = next.ChangeVersion(form.target.ID, form.target.Revision, version, loaderVersion)
		if err == nil && version != form.target.Version && len(a.releases) > 0 &&
			!slices.ContainsFunc(a.releases, func(v launcher.Version) bool { return v.ID == version }) {
			err = errors.New("官方版本列表中没有此版本")
		}
		checkID = form.target.ID
	case "remove":
		next.Remove(form.target.ID)
	}
	if err != nil {
		a.form.err = err.Error()
		return
	}
	if form.mode == "duplicate" && form.copyContent {
		if a.running != nil {
			a.form.err = "请先退出游戏，再复制资源与游戏设置"
			return
		}
		target, _ := next.Find(checkID)
		a.copyInstanceContent(form, next, target)
		return
	}
	a.form.err = ""
	a.save(next, checkID, func(err error) {
		same := a.form.mode == form.mode && a.form.target.ID == form.target.ID
		if err != nil {
			if same && a.form.open {
				a.form.err = err.Error()
			} else {
				a.message("保存实例失败", err)
			}
			return
		}
		visible := same && a.form.open
		if visible {
			a.form.open = false
		}
		switch form.mode {
		case "icon":
			if form.icon.picture == nil {
				delete(a.instanceIcons, form.target.ID)
			} else {
				if a.instanceIcons == nil {
					a.instanceIcons = make(map[int]ui.ImageSource)
				}
				a.instanceIcons[form.target.ID] = form.icon.picture
			}
		case "renameGroup":
			if a.libraryGroup != nil && *a.libraryGroup == form.originalGroup {
				name := strings.TrimSpace(form.name)
				a.libraryGroup = &name
			}
		case "group":
			if a.libraryGroup != nil && *a.libraryGroup != "" && !slices.Contains(next.GroupNames(), *a.libraryGroup) {
				a.libraryGroup = nil
			}
			if a.inspected == form.target.ID && !a.matchesGroup(a.inspected) {
				a.inspected = 0
			}
		case "duplicate":
			i, _ := a.state.Find(checkID)
			if visible {
				a.query, a.instanceFilter = "", 0
				a.inspect(i)
			} else {
				a.message("已创建实例 · "+i.Name, nil)
			}
		case "remove":
			delete(a.installed, form.target.ID)
			delete(a.instanceIcons, form.target.ID)
			delete(a.launchDrafts, form.target.ID)
			if a.inspected == form.target.ID {
				a.inspected = 0
			}
		case "version":
			a.message("版本已更换 · "+strings.TrimSpace(form.version), nil)
		case "loader":
			i, _ := a.state.Find(form.target.ID)
			a.message("已保存加载器 · "+loaderName(i), nil)
		}
	})
}

func (a *desktop) refreshCatalog() {
	if a.catalogBusy || a.ctx == nil {
		return
	}
	a.catalogBusy, a.catalogErr = true, ""
	go func() {
		ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
		defer cancel()
		versions, err := launcher.Releases(ctx)
		a.post(func() {
			a.catalogBusy = false
			if err != nil {
				a.catalogErr = "无法读取官方版本列表：" + err.Error()
			} else {
				a.releases = versions
			}
		})
	}()
}

func (a *desktop) settingsDraft() (launcher.Settings, error) {
	s := a.draft
	s.GameRoot = strings.TrimSpace(s.GameRoot)
	s.JavaPath = strings.TrimSpace(s.JavaPath)
	s.Player = strings.TrimSpace(s.Player)
	m, err := strconv.Atoi(strings.TrimSpace(a.memory))
	if err != nil {
		return s, errors.New("最大内存须为 1024–65536 MiB 的整数")
	}
	s.Memory = m
	return s, launcher.ValidateSettings(s)
}

func (a *desktop) saveSettings() {
	settings, err := a.settingsDraft()
	if err != nil {
		a.message("无法保存设置", err)
		return
	}
	next := a.state.Clone()
	rootChanged := settings.GameRoot != next.Settings.GameRoot
	previousPlayer := next.Settings.Player
	next.Settings = settings
	player := next.Settings.Player
	next.Settings.Player = previousPlayer
	_ = next.UseOfflinePlayer(player)
	next.MicrosoftProfile = a.state.MicrosoftProfile
	next.StationKey = a.state.StationKey
	a.save(next, 0, func(err error) {
		if err == nil {
			if rootChanged {
				a.install.err = ""
			}
			a.draft = next.Settings
			a.memory = strconv.Itoa(next.Settings.Memory)
			a.message("设置已保存", nil)
		} else {
			a.message("无法保存设置", err)
		}
	})
}

func (a *desktop) selectOfflinePlayer(name string) {
	name = strings.TrimSpace(name)
	browsed := a.browsedAccount
	next := a.state.Clone()
	if err := next.UseOfflinePlayer(name); err != nil {
		a.message("无法切换离线账号", err)
		return
	}
	a.save(next, 0, func(err error) {
		if err == nil {
			a.draft.Player = name
			if a.browsedAccount == browsed {
				a.browsedAccount = name
			}
			if browsed == addOfflineAccount {
				a.newOfflinePlayer = ""
			}
		}
		a.message("切换离线账号 · "+name, err)
	})
}

func (a *desktop) forgetOfflinePlayer(name string) {
	next := a.state.Clone()
	next.ForgetOfflinePlayer(name)
	a.save(next, 0, func(err error) {
		if err == nil && a.browsedAccount == name {
			a.browsedAccount = a.state.Settings.Player
		}
		a.message("移除离线账号 · "+name, err)
	})
}

func (a *desktop) detectJava() {
	if a.ctx == nil || a.busy || a.javaChecking {
		return
	}
	path := a.draft.JavaPath
	a.javaChecking, a.javaInfo, a.javaError = true, "", false
	a.writes.Go(func() {
		ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
		defer cancel()
		// The oldest supported game (1.20.1) uses Java 17. Per-instance
		// checks and launches apply the actual version metadata.
		j, err := launcher.FindJava(ctx, path, 17)
		a.post(func() {
			a.javaChecking = false
			if a.draft.JavaPath != path {
				return
			}
			if err != nil {
				a.javaInfo, a.javaError = err.Error(), true
				return
			}
			if strings.TrimSpace(path) != "" {
				a.draft.JavaPath = j.Path
			}
			a.javaInfo = fmt.Sprintf("Java %d · %s", j.Major, j.Path)
		})
	})
}

func (a *desktop) openPath(path string, create bool) {
	if !filepath.IsAbs(path) {
		a.message("无法打开路径", errors.New("需要绝对路径"))
		return
	}
	go func() {
		var err error
		if create {
			err = os.MkdirAll(path, 0755)
		}
		if err == nil {
			err = mygo.Shell.OpenPath(path)
		}
		if err != nil {
			a.post(func() { a.message("无法打开路径", err) })
		}
	}()
}

// Installation receipts are shared by game version, with extra loader-specific files.
func readInstallStatus(game launcher.Game, instances []launcher.Instance, version string) map[int]bool {
	status := make(map[int]bool)
	for _, i := range instances {
		if version == "" || i.Version == version {
			status[i.ID] = game.Installed(i)
		}
	}
	return status
}

func (a *desktop) play(i launcher.Instance, installOnly bool) {
	a.playOnServer(i, installOnly, "")
}

func (a *desktop) playOnServer(i launcher.Instance, installOnly bool, server string) {
	a.startGame(i, installOnly, server, nil)
}

func (a *desktop) startGame(i launcher.Instance, installOnly bool, server string, world *saves.World) {
	if a.busy || a.running != nil {
		return
	}
	a.busy, a.taskName = true, i.Name
	a.notice = ""
	a.task = launcher.Progress{Stage: "检查游戏文件"}
	ctx, cancel := context.WithCancel(a.ctx)
	a.cancelTask = cancel
	a.taskCanceling = false
	g := a.game
	request := (launcher.LaunchRequest{Instance: i, State: a.state, Account: a.microsoftAccount, Session: a.minecraftSession, StationSession: a.stationSessions[a.state.StationKey], Server: server, World: world, InstallOnly: installOnly}).Snapshot()
	instances := request.State.Instances
	record := taskRecord{InstanceID: i.ID, Revision: i.Revision, Name: i.Name, Version: i.Version, Action: taskLaunch, Server: server, At: time.Now()}
	if world != nil {
		record.World = world.Folder()
	}
	if installOnly {
		record.Action = taskVerify
	} else {
		record.Account = a.state.AccountKey()
	}
	record = record.Submitted()
	a.writes.Go(func() {
		defer cancel()
		var mu sync.Mutex
		var last time.Time
		var stage string
		report := func(p launcher.Progress) {
			mu.Lock()
			defer mu.Unlock()
			if p.Stage != stage || p.Done == p.Total || time.Since(last) > 140*time.Millisecond {
				last, stage = time.Now(), p.Stage
				a.post(func() {
					if a.task.Stage != p.Stage || a.task.Done <= p.Done {
						a.task = p
					}
				})
			}
		}
		result, err := g.Execute(ctx, a.store, request, report)
		run := result.Running
		var started map[int]playtime.Entry
		var startErr error
		if run.Done != nil {
			started, startErr = playtime.Record(a.store.Root, i.ID, run.StartedAt, 0)
		}
		account, session, stationSession, stationState := result.Account, result.Session, result.StationSession, result.State
		refreshed, stationStateChanged, authFailed := result.CredentialsChanged, result.StateChanged, result.AuthFailed
		installed := readInstallStatus(g, instances, i.Version)
		switch {
		case errors.Is(err, context.Canceled):
			record.Status, record.Detail = taskCanceled, "已校验的文件会在下次复用"
		case err != nil:
			record.Status, record.Detail = taskFailed, err.Error()
		case installOnly:
			record.Status, record.Detail = taskCompleted, "游戏文件已校验"
		default:
			record.Status, record.Log, record.PID = taskStarted, run.Log, run.PID
		}
		a.saveTask(record, func() {
			a.busy, a.cancelTask = false, nil
			a.taskCanceling = false
			a.task = launcher.Progress{}
			if refreshed {
				a.microsoftAccount, a.minecraftSession = account, session
			}
			if stationStateChanged {
				a.state = stationState
			}
			if stationSession.Profile.Key != "" {
				a.rememberStation(stationSession)
			}
			maps.Copy(a.installed, installed)
			switch {
			case errors.Is(err, context.Canceled):
				a.message("任务已取消，可继续下载", nil)
			case err != nil:
				a.message("任务失败 · "+i.Name, err)
				a.noticeJava = errors.Is(err, launcher.ErrJavaRequired)
				a.noticeAccount = authFailed || errors.Is(err, launcher.ErrMicrosoftRequired) || errors.Is(err, launcher.ErrSkinStationLogin)
				if a.noticeJava && i.Launch.JavaMode != launcher.JavaInherit {
					a.noticeJavaTarget = i
				}
			case installOnly:
				a.message("已校验游戏文件 · "+i.Name, nil)
			default:
				a.running, a.runningID, a.lastLog = &run, i.ID, run.Log
				a.playtimes, a.playtimeErr = started, startErr
				a.message(fmt.Sprintf("已启动 Java 进程 · PID %d", run.PID), nil)
			}
		})
		if run.Done == nil {
			return
		}
		exit, finished := run.Observe(a.ctx)
		played, playErr := playtime.Record(a.store.Root, i.ID, run.StartedAt, exit.At.Sub(run.StartedAt))
		if !finished {
			// Closing the launcher leaves the child running. Only the observed interval is known.
			if playErr != nil {
				log.Printf("save playtime for instance %d: %v", i.ID, playErr)
			}
			return
		}
		record.Status = taskExited
		if exit.Err != nil {
			record.Status, record.Detail = taskFailed, exit.Err.Error()
		}
		a.saveTask(record, func() {
			a.playtimes, a.playtimeErr = played, playErr
			a.running, a.runningID = nil, 0
			a.message("游戏已退出 · "+i.Name, exit.Err)
		})
	})
}

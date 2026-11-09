package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/layout"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/nav"
)

const (
	detailOverview = iota
	detailLaunch
	detailContent
	detailFiles
)

var instanceDetailTabs = []nav.TabItem{
	{Label: "概览", Icon: lucide.Package()},
	{Label: "启动设置", Icon: lucide.Settings2()},
	{Label: "资源", Icon: lucide.Layers()},
	{Label: "游戏文件", Icon: lucide.FolderOpen()},
}

type instanceLaunchFields struct {
	javaMode, javaPath, memory, display, width, height string
}

func launchFields(opts launcher.LaunchOptions) instanceLaunchFields {
	number := func(n int) string {
		if n == 0 {
			return ""
		}
		return strconv.Itoa(n)
	}
	return instanceLaunchFields{opts.JavaMode, opts.JavaPath, number(opts.Memory), opts.Display, number(opts.Width), number(opts.Height)}
}

func (f instanceLaunchFields) javaOptions() launcher.LaunchOptions {
	opts := launcher.LaunchOptions{JavaMode: f.javaMode}
	if f.javaMode == launcher.JavaCustom {
		opts.JavaPath = strings.Trim(strings.TrimSpace(f.javaPath), "\"")
	}
	return opts
}

func (f instanceLaunchFields) options() (launcher.LaunchOptions, error) {
	opts := f.javaOptions()
	opts.Display = f.display
	if value := strings.TrimSpace(f.memory); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil || n == 0 {
			return opts, errors.New("最大内存须为 1024–65536 MiB，留空则跟随全局")
		}
		opts.Memory = n
	}
	if opts.Display == launcher.DisplayResolution {
		opts.Width, _ = strconv.Atoi(strings.TrimSpace(f.width))
		opts.Height, _ = strconv.Atoi(strings.TrimSpace(f.height))
	}
	return opts, opts.Validate()
}

type instanceLaunchDraft struct {
	revision                 uint64
	fields                   instanceLaunchFields
	err, javaResult, javaKey string
	javaError, checking      bool
}

func (a *desktop) launchDraft(i launcher.Instance) *instanceLaunchDraft {
	if a.launchDrafts == nil {
		a.launchDrafts = make(map[int]*instanceLaunchDraft)
	}
	d := a.launchDrafts[i.ID]
	if d == nil || d.revision != i.Revision {
		d = &instanceLaunchDraft{revision: i.Revision, fields: launchFields(i.Launch)}
		a.launchDrafts[i.ID] = d
	}
	return d
}

func (a *desktop) instanceLaunchSettings(c *ui.Context, i launcher.Instance) {
	d := a.launchDraft(i)
	before := d.fields
	disabled := a.busy || a.pickerBusy
	layout.ScrollArea(c, layout.ScrollAreaProps{}, func() {
		ui.Column(c).Gap(10).FillWidth().MaxWidth(860).Children(func() {
			a.instanceRuntimeSettings(c, i, d, disabled)
			instanceDisplaySettings(c, &d.fields, disabled)
		})
	}).Grow(1).MinHeight(0)
	if before != d.fields {
		d.err = ""
	}
	if d.err != "" {
		feedback.Alert(c, d.err, feedback.AlertProps{Type: feedback.Error})
	}
	if d.fields != launchFields(i.Launch) {
		settingsActions(c, disabled, func() {
			d.fields, d.err, d.javaResult = launchFields(i.Launch), "", ""
		}, func() { a.saveInstanceLaunch(i, d, false) })
	}
}

var instanceJavaModes = []field.SelectOption{
	{Value: launcher.JavaInherit, Label: "跟随全局"},
	{Value: launcher.JavaAuto, Label: "自动选择"},
	{Value: launcher.JavaCustom, Label: "指定路径"},
}

func (a *desktop) instanceRuntimeSettings(c *ui.Context, i launcher.Instance, d *instanceLaunchDraft, disabled bool) {
	surface(c).Padding(10, 14).Gap(6).Children(func() {
		panelHeading(c, lucide.Coffee(), "Java 与内存", "")
		settingsRow(c, "Java 模式", func() {
			mode := d.fields.javaMode
			ui.Column(c).Grow(1).MaxWidth(220).Children(func() {
				field.Select(c, &d.fields.javaMode, field.SelectProps{Options: instanceJavaModes, Disabled: disabled}).Label("Java 模式")
			})
			if mode != d.fields.javaMode {
				d.fields.javaPath, d.javaResult = "", ""
			}
			if button(c, "检测 Java", action.Default, disabled) {
				a.checkInstanceJava(i, d)
			}
		})
		switch d.fields.javaMode {
		case launcher.JavaCustom:
			settingsRow(c, "Java 路径", func() {
				path := d.fields.javaPath
				settingsInput(c, "实例 Java 路径", &d.fields.javaPath, "选择 Java 可执行文件", 0, disabled)
				if path != d.fields.javaPath {
					d.javaResult = ""
				}
				if button(c, "浏览…", action.Default, disabled) {
					a.pickJavaPath(d.fields.javaPath, func(path string) {
						d.fields.javaPath, d.javaResult, d.err = path, "", ""
					})
				}
			})
		case launcher.JavaInherit:
			if path := a.state.Settings.JavaPath; path != "" {
				settingsRow(c, "全局 Java", func() {
					muted(c, path).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").Selectable()
				})
			}
		}
		if d.checking {
			muted(c, "正在检测 Java…").FontSize(12)
		} else if d.javaResult != "" && d.javaKey == d.fields.javaOptions().JavaExecutable(a.state.Settings.JavaPath) {
			tone := feedback.Success
			if d.javaError {
				tone = feedback.Error
			}
			feedback.Alert(c, d.javaResult, feedback.AlertProps{Type: tone})
		}
		layout.Divider(c, layout.DividerProps{})
		settingsRow(c, "最大内存（MiB）", func() {
			placeholder := fmt.Sprintf("%d（全局）", a.state.Settings.Memory)
			settingsInput(c, "实例最大内存", &d.fields.memory, placeholder, 126, disabled)
			if button(c, "4 GiB", action.Default, disabled || d.fields.memory == "4096") {
				d.fields.memory = "4096"
			}
			if button(c, "8 GiB", action.Default, disabled || d.fields.memory == "8192") {
				d.fields.memory = "8192"
			}
			if button(c, "跟随全局", action.Text, disabled || d.fields.memory == "") {
				d.fields.memory = ""
			}
		})
	})
}

var instanceDisplayModes = []field.SelectOption{
	{Value: launcher.DisplayDefault, Label: "游戏默认"},
	{Value: launcher.DisplayResolution, Label: "自定义分辨率"},
	{Value: launcher.DisplayFullscreen, Label: "启动时全屏"},
}

func instanceDisplaySettings(c *ui.Context, f *instanceLaunchFields, disabled bool) {
	surface(c).Padding(10, 14).Gap(6).Children(func() {
		panelHeading(c, lucide.Monitor(), "游戏窗口", "")
		settingsRow(c, "显示模式", func() {
			prev := f.display
			ui.Column(c).Grow(1).MaxWidth(220).Children(func() {
				field.Select(c, &f.display, field.SelectProps{Options: instanceDisplayModes, Disabled: disabled}).Label("显示模式")
			})
			if prev != f.display {
				if f.display == launcher.DisplayResolution {
					f.width, f.height = "1280", "720"
				} else {
					f.width, f.height = "", ""
				}
			}
		})
		if f.display == launcher.DisplayResolution {
			settingsRow(c, "分辨率（像素）", func() {
				settingsInput(c, "窗口宽度", &f.width, "宽度", 96, disabled)
				muted(c, "×")
				settingsInput(c, "窗口高度", &f.height, "高度", 96, disabled)
			})
		}
	})
}

func (a *desktop) saveInstanceLaunch(i launcher.Instance, d *instanceLaunchDraft, start bool) {
	opts, err := d.fields.options()
	next := a.state.Clone()
	if err == nil {
		err = next.ConfigureLaunch(i.ID, d.revision, opts)
	}
	if err != nil {
		d.err = err.Error()
		return
	}
	a.save(next, 0, func(err error) {
		if err != nil {
			d.err = "无法保存启动设置：" + err.Error()
			return
		}
		delete(a.launchDrafts, i.ID)
		if start {
			saved, _ := a.state.Find(i.ID)
			a.play(saved, false)
		} else {
			a.message("启动设置已保存 · "+i.Name, nil)
		}
	})
}

func (a *desktop) checkInstanceJava(i launcher.Instance, d *instanceLaunchDraft) {
	i.Launch = d.fields.javaOptions()
	globalPath := a.state.Settings.JavaPath
	d.javaKey = i.Launch.JavaExecutable(globalPath)
	d.javaResult = ""
	if err := i.Launch.Validate(); err != nil {
		d.javaError, d.javaResult = true, err.Error()
		return
	}
	a.busy, d.checking = true, true
	a.writes.Go(func() {
		ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
		defer cancel()
		j, required, err := a.game.CheckJava(ctx, i, globalPath)
		a.post(func() {
			a.busy, d.checking = false, false
			d.javaError = err != nil
			if err != nil {
				d.javaResult = err.Error()
			} else {
				d.javaResult = fmt.Sprintf("Java %d · 需要 Java %d+ · %s", j.Major, required, j.Path)
			}
		})
	})
}

var instanceFolders = []struct{ label, path string }{
	{"存档", "saves"},
	{"资源包", "resourcepacks"},
	{"截图", "screenshots"},
	{"崩溃报告", "crash-reports"},
	{"启动日志", "launcher-logs"},
}

func (a *desktop) instanceFiles(c *ui.Context, i launcher.Instance) {
	disabled := a.busy || a.pickerBusy || a.running != nil
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Spacer(c).Grow(1)
		dir := launcher.BackupsDir(a.store.Root, i.ID)
		if button(c, "打开备份目录", action.Text, false) {
			a.openPath(dir, true)
		}
		if button(c, "恢复存档…", action.Default, disabled) {
			a.pickPath(mygo.OpenDialogOptions{Title: "选择存档 ZIP（保留已有世界）", DefaultPath: dir,
				Filters: []mygo.FileFilter{{Name: "存档归档", Extensions: []string{"zip"}}}}, func(path string) {
				a.runInstanceTask(i, "恢复存档", func(ctx context.Context) error {
					_, err := launcher.RestoreSaves(ctx, i, path)
					return err
				})
			})
		}
		if button(c, "备份存档", action.Primary, disabled) {
			root := a.store.Root
			a.runInstanceTask(i, "备份存档", func(ctx context.Context) error {
				_, err := launcher.BackupSaves(ctx, i, root)
				return err
			})
		}
	})
	layout.ScrollArea(c, layout.ScrollAreaProps{}, func() {
		surface(c).Padding(10, 14).Gap(5).Children(func() {
			for idx, folder := range instanceFolders {
				if idx != 0 {
					layout.Divider(c, layout.DividerProps{})
				}
				path := filepath.Join(i.GameDir, folder.path)
				settingsRow(c.Key(folder.path), folder.label, func() {
					muted(c, folder.path).Grow(1)
					action.CopyButton(c, path, action.CopyButtonProps{}).Label("复制" + folder.label + "目录")
					if icon(c, lucide.FolderOpen(), "打开"+folder.label+"目录", false) {
						a.openPath(path, true)
					}
				})
			}
			if path := a.latestInstanceLog(i); path != "" {
				layout.Divider(c, layout.DividerProps{})
				if button(c, "查看最近日志", action.Default, false) {
					a.showLog(path)
				}
			}
		})
	}).Grow(1).MinHeight(0)
}

func (a *desktop) latestInstanceLog(i launcher.Instance) string {
	dir := filepath.Join(i.GameDir, "launcher-logs")
	for _, record := range a.history {
		if (record.InstanceID == i.ID || record.InstanceID == 0) && isGameLog(record.Log) && filepath.Dir(record.Log) == dir {
			return record.Log
		}
	}
	return ""
}

package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/CAUCraft/LXMCL/internal/shortcut"
	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/egoist/mygo"
)

func (a *desktop) chooseLaunchShortcut(i launcher.Instance) {
	if a.ctx == nil || a.pickerBusy || a.busy {
		return
	}
	name := i.Name + ".lnk"
	if !storage.ValidFilename(name) {
		name = fmt.Sprintf("LXMCL-%d.lnk", i.ID)
	}
	options := mygo.SaveDialogOptions{Parent: a.window, Title: "创建启动快捷方式", ButtonLabel: "创建", DefaultPath: name,
		Filters: []mygo.FileFilter{{Name: "Windows 快捷方式", Extensions: []string{"lnk"}}}}
	a.pickerBusy = true
	go func() {
		path, err := mygo.Dialog.Save(options)
		a.post(func() {
			a.pickerBusy = false
			if err != nil {
				a.message("无法选择快捷方式位置", err)
				return
			}
			if path == "" {
				return
			}
			current, ok := a.state.Find(i.ID)
			if !ok || a.busy {
				a.message("无法创建快捷方式", errors.New("请在任务结束后重新选择实例"))
				return
			}
			profile := a.store.Root
			a.writes.Go(func() {
				executable, err := os.Executable()
				if err == nil {
					err = shortcut.Create(a.ctx, path, executable, profile, current.ID)
				}
				a.post(func() {
					switch {
					case errors.Is(err, fs.ErrExist):
						a.message("快捷方式已存在", errors.New("请更换文件名或保存位置"))
					case err != nil:
						a.message("无法创建启动快捷方式", err)
					default:
						a.message("已创建启动快捷方式 · "+current.Name, nil)
					}
				})
			})
		})
	}()
}

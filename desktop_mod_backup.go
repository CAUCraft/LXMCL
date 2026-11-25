package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo"
)

func (a *desktop) chooseModBackup(i launcher.Instance, files []launcher.ContentFile) {
	if len(files) == 0 || a.ctx == nil || a.busy || a.pickerBusy || a.running != nil {
		return
	}
	request := a.contentRequest
	a.pickModBackupDestination(i, func(path string) {
		current, ok := a.state.Find(i.ID)
		viewChanged := a.contentRequest != request || a.page != pageLibrary || a.inspected != i.ID || a.detailPage != detailContent
		if !ok || current != i || viewChanged || a.busy || a.running != nil {
			a.message("无法备份模组", errors.New("实例状态已变化，请重新选择模组"))
			return
		}
		row := taskRecord{
			InstanceID: i.ID, Revision: i.Revision, Name: i.Name, Version: i.Version,
			Action: fmt.Sprintf("备份 %d 项模组", len(files)), Resource: &resourceTask{Kind: string(launcher.Mods)},
		}
		a.runFileTask(row, func(ctx context.Context, report func(launcher.Progress)) error {
			return content.BackupMods(ctx, i, files, path, report)
		}, nil)
	})
}

func (a *desktop) pickModBackupDestination(i launcher.Instance, accept func(string)) {
	options := mygo.SaveDialogOptions{
		Parent: a.window, Title: "备份所选模组", ButtonLabel: "备份",
		DefaultPath: fmt.Sprintf("instance-%d-mods.zip", i.ID),
		Filters:     []mygo.FileFilter{{Name: "模组备份 ZIP", Extensions: []string{"zip"}}},
	}
	a.pickerBusy = true
	go func() {
		path, err := mygo.Dialog.Save(options)
		a.post(func() {
			a.pickerBusy = false
			if err != nil {
				a.message("无法选择备份位置", err)
				return
			}
			if path != "" {
				accept(path)
			}
		})
	}()
}

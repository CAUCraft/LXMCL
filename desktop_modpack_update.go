package main

import (
	"context"
	"fmt"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

func (a *desktop) packChangeList(c *ui.Context, p *modpackView, i launcher.Instance) {
	display.Text(c, fmt.Sprintf("当前实例 · %s · %s", i.Version, loaderName(i)), display.TextProps{Strong: true})
	muted(c, "仅更新包管理的模组、资源和配置。世界与本地修改会保留。").FontSize(12)
	if p.update == nil {
		return
	}
	u := p.update
	if !u.Tracked {
		muted(c, "此实例尚无整合包记录。首次更新会保留已有文件，补齐缺少的文件。").FontSize(12)
	} else if u.Previous != "" {
		muted(c, "已安装整合包版本 · "+u.Previous).FontSize(12)
	}
	if len(u.Changes) == 0 {
		muted(c, "资源文件已一致。应用后校验游戏版本并保存整合包记录。")
		return
	}
	labels := map[string]string{"keep": "保留本地", "replace": "替换", "add": "新增", "remove": "移除"}
	colors := map[string]display.TagColor{"keep": display.TagGray, "replace": display.TagBlue, "add": display.TagGreen, "remove": display.TagGold}
	display.Text(c, fmt.Sprintf("文件变更 (%d)", len(u.Changes)), display.TextProps{Strong: true})
	const size = 12
	start := p.changePage * size
	for _, entry := range u.Changes[start:min(start+size, len(u.Changes))] {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			display.Tag(c, labels[entry.Action], display.TagProps{Color: colors[entry.Action], Size: theme.Small})
			display.Text(c, entry.Path, display.TextProps{FontSize: 13}).Grow(1).MinWidth(0).Selectable()
		})
	}
	if len(u.Changes) > size {
		ui.Row(c).Gap(8).Children(func() {
			if button(c, "上一页", action.Default, start == 0) {
				p.changePage--
			}
			muted(c, fmt.Sprintf("%d / %d", p.changePage+1, (len(u.Changes)+size-1)/size))
			if button(c, "下一页", action.Default, start+size >= len(u.Changes)) {
				p.changePage++
			}
		})
	}
}
func (a *desktop) previewPackUpdate(p *modpackView, i launcher.Instance) {
	game, pack, key := a.game, p.prepared, p.key
	var preview launcher.PackUpdate
	row := taskRecord{InstanceID: i.ID, Revision: i.Revision, Version: i.Version, Name: i.Name, Action: "预览整合包更新", Pack: launcher.PackUpdateReference(i, pack)}
	a.runFileTask(row, func(ctx context.Context, _ func(launcher.Progress)) error {
		var err error
		preview, err = game.PreviewPackUpdate(ctx, i, pack, key)
		return err
	}, func(err error) {
		if pack != p.prepared {
			return
		}
		if err != nil {
			p.setInstallError(err, key)
			return
		}
		p.changePage, p.update, p.installErr = 0, &preview, nil
	})
}
func (a *desktop) applyPackUpdate(p *modpackView, i launcher.Instance) {
	game, store, state, pack, key, u := a.game, a.store, a.state.Clone(), p.prepared, p.key, *p.update
	var next launcher.State
	row := taskRecord{InstanceID: i.ID, Revision: i.Revision, Version: i.Version, Name: i.Name, Action: "更新整合包", Pack: launcher.PackUpdateReference(i, pack)}
	a.runFileTask(row, func(ctx context.Context, report func(launcher.Progress)) error {
		var err error
		next, err = game.ApplyPackUpdate(ctx, store, state, i, pack, key, u, report)
		return err
	}, func(err error) {
		if err == nil {
			a.state = next
			if a.installed == nil {
				a.installed = make(map[int]bool)
			}
			a.installed[i.ID] = true
		}
		if p.prepared != pack {
			return
		}
		p.update = nil
		if err != nil {
			p.setInstallError(err, key)
			return
		}
		p.clearPreview()
	})
}

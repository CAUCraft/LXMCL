package main

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/field"
)

type packRetry struct {
	row        taskRecord
	askFeed    bool
	path, feed string
}

func (a *desktop) openPackRetry(row taskRecord) {
	i, ok := launcher.PackRetryTarget(row, a.state)
	if !ok || a.ctx == nil || a.busy || a.running != nil {
		return
	}
	p := &a.modpacks
	p.stop()
	p.clearPreview()
	r := &packRetry{row: row, path: row.Pack.Archive}
	for _, address := range []string{i.PackFeed, p.feedURL} {
		if launcher.MatchesPackFeed(row.Pack, address) {
			r.feed = address
			break
		}
	}
	a.page, a.downloadPage, p.mode = pageDownloads, downloadModpacks, modpackLocal
	r.askFeed = !launcher.MatchesPackFeed(row.Pack, r.feed)
	p.retry = r
	p.archive = filepath.Base(r.path)
	p.target = strconv.Itoa(i.ID)
	if launcher.MatchesPackFeed(row.Pack, r.feed) {
		a.readPackRetry(p, r)
	}
}
func (a *desktop) readPackRetry(p *modpackView, r *packRetry) {
	if a.busy || a.ctx == nil || p.retry != r {
		return
	}
	i, ok := launcher.PackRetryTarget(r.row, a.state)
	if !ok {
		p.prepareErr = "原实例已变更，请从实例重新选择整合包"
		return
	}
	ctx, cancel, request := p.begin(a.ctx, 6*time.Minute)
	p.preparing, p.prepareErr = true, ""
	file, feed, ref := r.path, r.feed, r.row.Pack
	go func() {
		defer cancel()
		pack, err := launcher.ReopenPackUpdate(ctx, ref, file, feed)
		a.post(func() {
			if p.request != request || p.retry != r {
				return
			}
			p.preparing = false
			if err != nil {
				p.prepareErr = err.Error()
				return
			}
			if _, ok := launcher.PackRetryTarget(r.row, a.state); !ok {
				p.prepareErr = "原实例已变更，请从实例重新选择整合包"
				return
			}
			p.prepared = pack
			info := pack.Info()
			if !a.busy && a.running == nil && (info.Source != launcher.CurseForgeModpacks || info.Downloads == 0 || p.key != "") {
				a.previewPackUpdate(p, i)
			}
		})
	}()
}
func (a *desktop) packRetryInput(c *ui.Context, p *modpackView) {
	r := p.retry
	heading(c, "重新预览整合包更新")
	display.Text(c, r.row.Name+" · "+r.row.Version, display.TextProps{Strong: true})
	muted(c, r.path).FontSize(12).SingleLine().Ellipsis("…").Tooltip(r.path).Selectable()
	if p.prepareErr != "" {
		feedback.Alert(c, "无法重新读取整合包", feedback.AlertProps{Type: feedback.Error})
		ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
			muted(c, p.prepareErr).FontSize(12).MaxLines(2).Ellipsis("…").Grow(1).MinWidth(0).Selectable()
			action.CopyButton(c, p.prepareErr, action.CopyButtonProps{}).Label("复制整合包读取错误")
		})
	}
	if _, ok := launcher.PackRetryTarget(r.row, a.state); !ok {
		muted(c, "原实例已变更，请重新选择更新目标。")
		return
	}
	if r.askFeed {
		field.Input(c, &r.feed, field.InputProps{Placeholder: "原服主订阅地址"}).Label("原服主订阅地址")
	}
	locked := a.busy || a.pickerBusy || !launcher.MatchesPackFeed(r.row.Pack, r.feed)
	ui.Row(c).Gap(8).Children(func() {
		if button(c, "重新读取", action.Primary, locked) {
			a.readPackRetry(p, r)
		}
		if button(c, "定位原整合包…", action.Default, locked) {
			a.pickPath(mygo.OpenDialogOptions{Title: "选择内容相同的原整合包", Filters: []mygo.FileFilter{{Name: "整合包", Extensions: []string{"mrpack", "zip"}}}}, func(path string) {
				if p.retry == r {
					r.path = path
					a.readPackRetry(p, r)
				}
			})
		}
	})
}

func (a *desktop) openPackBackup(row taskRecord) {
	if !launcher.CanOpenPackBackup(row.Pack) {
		return
	}
	path := row.Pack.BackupDir
	go func() {
		st, err := os.Lstat(path)
		if err == nil {
			if st.IsDir() && st.Mode()&os.ModeSymlink == 0 {
				err = mygo.Shell.OpenPath(path)
			} else {
				err = errors.New("原备份目录已变更")
			}
		}
		if err != nil {
			a.post(func() { a.message("无法打开回滚备份", err) })
		}
	}()
}

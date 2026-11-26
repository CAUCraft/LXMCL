package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"

	"github.com/CAUCraft/LXMCL/internal/config"
	"github.com/CAUCraft/LXMCL/internal/server"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
)

const (
	serverBackupExport = iota
	serverBackupRestore
	serverBackupGameList
)

type serverBackupView struct {
	mode      int
	path, err string
	rows      []server.Server
	plan      []config.ServerImportEntry
	added     int
	list      ui.ListState
	working   bool
	gameList  *serverGameExport
}

func (a *desktop) serverBackupDialog(c *ui.Context) {
	p := a.serverBackup
	if p == nil {
		return
	}
	open := true
	locked := p.working || a.pickerBusy || a.busy
	feedback.Modal(c, &open, feedback.ModalProps{Title: "服务器收藏备份", Width: 510, NoFooter: true}, func() {
		ui.Column(c).Gap(10).Children(func() {
			last := p.mode
			action.Segmented(c, &p.mode, []string{"导出收藏", "恢复收藏", "导出 servers.dat"}, action.SegmentedProps{}).Label("备份操作").Disabled(locked)
			if last != p.mode {
				p.path, p.rows, p.plan, p.added, p.err = "", nil, nil, 0, ""
				p.list, p.gameList = ui.ListState{}, nil
			}
			if p.mode == serverBackupGameList {
				a.serverGameExportView(c, p, &open, locked)
				return
			}
			rows := a.state.Servers
			if p.mode == serverBackupRestore {
				rows = p.rows
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					if button(c, "选择备份文件…", action.Default, locked) {
						a.chooseFavoritesBackup(p)
					}
					if p.path != "" {
						muted(c, filepath.Base(p.path)).MinWidth(0).Grow(1).SingleLine().Ellipsis("…").Tooltip(p.path)
					}
				})
			}
			switch {
			case p.mode == serverBackupExport:
				muted(c, fmt.Sprintf("将导出全部 %d 条启动器收藏", len(rows)))
			case p.working:
				flightLoading(c, "正在处理收藏备份…")
			case rows != nil:
				muted(c, fmt.Sprintf("将追加 %d 条，跳过 %d 条同址收藏", p.added, len(rows)-p.added))
			}
			if len(rows) != 0 {
				ui.List(c.Key(p.mode), &p.list, len(rows)).FillWidth().Height(float32(min(len(rows), 5) * 38)).Rows(func(row ui.ListRow) {
					entry := rows[row.Index]
					rc := row.Context
					ui.Row(rc).Height(38).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Column(rc).Grow(1).MinWidth(0).Children(func() {
							ui.Text(rc, entry.Name).SingleLine().Ellipsis("…")
							muted(rc, entry.Address).FontSize(12).SingleLine().Ellipsis("…").Tooltip(entry.Address)
						})
						if p.mode == serverBackupRestore {
							status := "跳过同址"
							if p.plan[row.Index].Add {
								status = "将追加"
							}
							muted(rc, status).FontSize(12)
						}
					})
				})
			}
			if p.err != "" {
				feedback.Alert(c, "无法处理收藏备份", feedback.AlertProps{Type: feedback.Warning, Description: p.err})
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if button(c, "取消", action.Default, locked) {
					open = false
				}
				if p.mode == serverBackupExport {
					if button(c, "导出备份…", action.Primary, locked || len(rows) == 0) {
						a.exportFavoritesBackup(p)
					}
				} else if button(c, "确认追加", action.Primary, locked || p.added == 0) {
					a.restoreFavoritesBackup(p)
				}
			})
		})
	})
	if !open && !locked && a.serverBackup == p {
		a.serverBackup = nil
	}
}

func (a *desktop) chooseFavoritesBackup(p *serverBackupView) {
	a.pickPath(mygo.OpenDialogOptions{Title: "选择服务器收藏备份", ButtonLabel: "预览备份", Filters: []mygo.FileFilter{{Name: "收藏备份", Extensions: []string{"json"}}}}, func(path string) {
		if a.serverBackup != p || p.mode != serverBackupRestore {
			return
		}
		p.path, p.rows, p.plan, p.added, p.err = path, nil, nil, 0, ""
		p.working = true
		p.list = ui.ListState{}
		go func() {
			rows, err := server.ReadFavorites(path)
			a.post(func() {
				p.working = false
				if a.serverBackup != p || p.mode != serverBackupRestore {
					return
				}
				if err != nil {
					p.err = err.Error()
					return
				}
				p.rows = rows
				p.plan = a.state.PlanServerImport(rows)
				for _, row := range p.plan {
					if row.Add {
						p.added++
					}
				}
			})
		}()
	})
}

func (a *desktop) exportFavoritesBackup(p *serverBackupView) {
	options := mygo.SaveDialogOptions{Title: "导出服务器收藏备份", ButtonLabel: "导出", DefaultPath: "lxmcl-servers.json", Filters: []mygo.FileFilter{{Name: "收藏备份", Extensions: []string{"json"}}}}
	a.saveServerExport(p, options, slices.Clone(a.state.Servers), server.ExportFavorites, "收藏备份已导出")
}

func (a *desktop) saveServerExport(p *serverBackupView, options mygo.SaveDialogOptions, rows []server.Server, write func(context.Context, string, []server.Server) error, success string) {
	if a.ctx == nil || a.pickerBusy || a.busy || p.working || a.serverBackup != p || len(rows) == 0 {
		return
	}
	p.err, p.working, a.pickerBusy = "", true, true
	options.Parent = a.window
	go func() {
		path, err := mygo.Dialog.Save(options)
		if err == nil && path != "" {
			err = write(a.ctx, path, rows)
		}
		a.post(func() {
			p.working, a.pickerBusy = false, false
			if a.serverBackup != p {
				return
			}
			if errors.Is(err, fs.ErrExist) {
				p.err = "已有同名文件，请更换文件名或目录。"
			} else if err != nil {
				p.err = err.Error()
			} else if path != "" {
				a.serverBackup = nil
				a.message(success+" · "+filepath.Base(path), nil)
			}
		})
	}()
}

func (a *desktop) restoreFavoritesBackup(p *serverBackupView) {
	next := a.state.Clone()
	added := next.ImportServers(p.rows)
	if added == 0 {
		a.serverBackup = nil
		a.message("这些服务器均已收藏", nil)
		return
	}
	p.err = ""
	a.save(next, 0, func(err error) {
		if err != nil {
			p.err = err.Error()
			return
		}
		a.serverBackup = nil
		a.message(fmt.Sprintf("已恢复 %d 条服务器收藏", added), nil)
	})
}

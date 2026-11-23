package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
)

func formatPlaytime(millis int64) string {
	minutes := millis / 60000
	switch {
	case minutes == 0:
		return "不足 1 分钟"
	case minutes < 60:
		return fmt.Sprintf("%d 分钟", minutes)
	default:
		return fmt.Sprintf("%d 小时 %d 分钟", minutes/60, minutes%60)
	}
}

func (a *desktop) playtimeOverview(c *ui.Context, id int) {
	if a.playtimeErr != nil {
		feedback.Alert(c, "游玩记录读写失败："+a.playtimeErr.Error(), feedback.AlertProps{Type: feedback.Error})
		return
	}
	e := a.playtimes[id]
	if e.LastPlayed.IsZero() {
		property(c, "游玩记录", "暂无游玩记录")
		return
	}
	property(c, "上次游玩", e.LastPlayed.Local().Format("2006-01-02 15:04"))
	total := formatPlaytime(e.TotalMillis())
	if a.runningID == id {
		total += " · 本次结束后更新"
	}
	property(c, "累计游玩", total)
}

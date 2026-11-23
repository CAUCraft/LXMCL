package main

import (
	"testing"
	"time"

	"github.com/CAUCraft/LXMCL/internal/playtime"
)

func TestPlaytimeOverviewUsesOnlyTheInspectedInstance(t *testing.T) {
	a, tt := testDesktop(t)
	started := time.Date(2026, 5, 12, 9, 8, 0, 0, time.Local)
	home, other := a.state.Instances[0], a.state.Instances[1]
	a.history = []taskRecord{{InstanceID: other.ID, Action: taskLaunch, Status: taskExited, At: started}}
	a.playtimes = map[int]playtime.Entry{home.ID: {LastPlayed: started, SessionMillis: (92 * time.Minute).Milliseconds()}}
	tt.SetSize(820, 530)
	a.inspect(other)
	tt.Frame()
	if !tt.HasText("暂无游玩记录") || tt.HasText("1 小时 32 分钟") {
		t.Fatal(tt.Texts())
	}
	a.inspect(home)
	tt.Frame()
	if !tt.HasText("2026-05-12 09:08") || !tt.HasText("1 小时 32 分钟") {
		t.Fatal(tt.Texts())
	}
}

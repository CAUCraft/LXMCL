package main

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/layout"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
)

func flightLoading(c *ui.Context, label string) {
	ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
		flightIndicator(c)
		muted(c, label).FontSize(13)
	})
}

func flightIndicator(c *ui.Context) {
	ink, wash := brand(c)
	track := ui.Box(c).Size(84, 26).Shrink(0).Clip().Radius(7).Background(wash)
	k := float32(0.52)
	if !c.Preferences().ReduceMotion {
		k = track.Keyframes("flight", []ui.Keyframe{{Value: 0}, {At: 1900 * time.Millisecond, Value: 1}},
			ui.KeyframeOptions{Repeat: ui.RepeatForever})
	}
	lift := float32(math.Sin(float64(k) * math.Pi))
	track.Children(func() {
		for i := range 7 {
			ui.Box(c.Key(i)).Absolute().Top(19).Left(float32(8+10*i)).Size(5, 1).Background(ink.Alpha(0.35))
		}
		ui.Icon(c, lucide.Plane()).Absolute().Top(5).Left(5).FontSize(16).TextColor(ink).
			Translate(k*58, 4-lift*6).Rotate(26 - 16*lift).Opacity(min(1, 0.25+4*min(k, 1-k)))
	})
}

func splitWorkspace(c *ui.Context, width float32, left, right func()) {
	ui.Row(c).Gap(12).Grow(1).Basis(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
		sectionPane(c).Width(width).Shrink(0).MinHeight(0).Padding(0, 2).Gap(8).Children(left)
		layout.Divider(c, layout.DividerProps{Vertical: true}).FillHeight().AlignSelf(ui.Stretch)
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).MinHeight(0).Gap(10).Children(right)
	})
}

func listWidth(window float32) float32 {
	return min(252, max(178, window*0.22))
}

func detailPane(c *ui.Context, content func()) {
	ui.Column(c).Grow(1).Basis(0).MinHeight(0).MinWidth(0).Gap(10).Children(content)
}

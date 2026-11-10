package main

import (
	"image"
	"math"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/layout"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

const frontYaw, frontPitch, backYaw = -0.48, -0.20, math.Pi + 0.37

type modelAngle struct{ yaw, pitch float64 }

type modelOrbit struct {
	modelAngle
	from  modelAngle
	start time.Time
}

func frontOrbit() modelOrbit { return modelOrbit{modelAngle: modelAngle{frontYaw, frontPitch}} }
func backOrbit() modelAngle  { return modelAngle{backYaw, frontPitch} }

func (o *modelOrbit) turn(c *ui.Context, target modelAngle) {
	o.from, _ = o.sample(c.Now())
	target.yaw = o.from.yaw + math.Remainder(target.yaw-o.from.yaw, 2*math.Pi)
	o.modelAngle, o.start = target, time.Time{}
	if !c.Preferences().ReduceMotion && o.from != target {
		o.start = c.Now()
	}
	c.Invalidate()
}

func (o *modelOrbit) sample(now time.Time) (modelAngle, bool) {
	const duration = 260 * time.Millisecond
	if o.start.IsZero() || now.Sub(o.start) >= duration {
		return o.modelAngle, false
	}
	k := max(0, float64(now.Sub(o.start))/float64(duration))
	k = k * k * (3 - 2*k)
	return modelAngle{o.from.yaw + (o.yaw-o.from.yaw)*k, o.from.pitch + (o.pitch-o.from.pitch)*k}, true
}

type modelFrame struct {
	skin, cape *image.NRGBA
	slim       bool
	angle      modelAngle
	faces      []skinFace
	bitmap     *ui.Bitmap
	buf        [2]*image.RGBA
	depth      []float64
	next       int
}

func (f *modelFrame) image(skin, cape *image.NRGBA, slim bool, angle modelAngle) *ui.Bitmap {
	newModel := f.skin != skin || f.cape != cape || f.slim != slim
	if f.bitmap != nil && !newModel && f.angle == angle {
		return f.bitmap
	}
	if newModel {
		f.skin, f.cape, f.slim = skin, cape, slim
		f.faces = skinModel(skin, cape, slim)
	}
	f.angle = angle
	if f.depth == nil {
		f.depth = make([]float64, previewWidth*previewHeight)
		for i := range f.buf {
			f.buf[i] = image.NewRGBA(image.Rect(0, 0, previewWidth, previewHeight))
		}
	}
	target := f.buf[f.next]
	f.next = 1 - f.next
	drawSkinModel(target, f.depth, f.faces, angle.yaw, angle.pitch)
	if f.bitmap == nil {
		f.bitmap = ui.NewBitmap(target)
	} else {
		f.bitmap.Update(target)
	}
	return f.bitmap
}

func modelPreview(c *ui.Context, skin skinPreview, cape *image.NRGBA, slim bool, orbit *modelOrbit, caption string) {
	sectionPane(c).Grow(1).Basis(0).MinWidth(0).MinHeight(0).Gap(7).Children(func() {
		tk := theme.TokensOf(c)
		ui.Row(c).Padding(2, 4).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, lucide.Box()).FontSize(14).TextColor(tk.TextSecondary)
			display.Text(c, "3D 预览", display.TextProps{Strong: true, FontSize: 13})
			if caption != "" {
				ui.Spacer(c).Grow(1)
				muted(c, caption).FontSize(11).MinWidth(0).SingleLine().Ellipsis("…")
			}
		})
		modelStage(c, skin.atlas, cape, slim, orbit)
		ui.Row(c).Padding(0, 3).Gap(3).AlignItems(ui.Center).Children(func() {
			muted(c, "拖动旋转").FontSize(12).Grow(1).MinWidth(0)
			if icon(c, lucide.ArrowLeft(), "向左旋转", false) {
				orbit.turn(c, modelAngle{orbit.yaw - math.Pi/6, orbit.pitch})
			}
			if icon(c, lucide.ArrowRight(), "向右旋转", false) {
				orbit.turn(c, modelAngle{orbit.yaw + math.Pi/6, orbit.pitch})
			}
			if action.Button(c, "背面", action.ButtonProps{Variant: action.Text, Size: theme.Small}).Label("查看背面").Clicked() {
				orbit.turn(c, backOrbit())
			}
			if action.Button(c, "复位", action.ButtonProps{Variant: action.Text, Size: theme.Small}).Label("重置视角").Clicked() {
				orbit.turn(c, frontOrbit().modelAngle)
			}
		})
	})
}

func modelStage(c *ui.Context, skin, cape *image.NRGBA, slim bool, orbit *modelOrbit) {
	// The only custom surface is the viewport. Lobe supplies the surrounding
	// controls and MyGo handles pointer capture and the texture display.
	_, wash := brand(c)
	stage := layout.Block(c, layout.BlockProps{Variant: layout.BlockFilled}).
		Grow(1).Basis(0).MinHeight(0).FillWidth().Clip().Background(wash).
		Label("角色模型").Focusable().Cursor(ui.CursorGrab)
	if c.Preferences().ReduceMotion {
		orbit.start = time.Time{}
	}
	angle, moving := orbit.sample(c.Now())
	if dx, dy, held := stage.Dragged(); held {
		stage.Cursor(ui.CursorGrabbing)
		angle.yaw = math.Remainder(angle.yaw+float64(dx)*0.014, 2*math.Pi)
		angle.pitch = max(-0.55, min(0.55, angle.pitch+float64(dy)*0.01))
		orbit.modelAngle, orbit.start, moving = angle, time.Time{}, false
	}
	if stage.Shortcut(0, ui.KeyLeft) {
		orbit.turn(c, modelAngle{orbit.yaw - math.Pi/12, orbit.pitch})
	}
	if stage.Shortcut(0, ui.KeyRight) {
		orbit.turn(c, modelAngle{orbit.yaw + math.Pi/12, orbit.pitch})
	}
	if stage.Shortcut(0, ui.KeyHome) {
		orbit.turn(c, frontOrbit().modelAngle)
	}
	if moving {
		c.AnimationFrame()
	}
	frame := ui.Local(stage, "frame", func() modelFrame { return modelFrame{} })
	model := frame.image(skin, cape, slim, angle)
	stage.Draw(func(p *ui.Painter, r ui.Rect) {
		h := min(r.H-8, r.W*1.1)
		if h <= 0 {
			return
		}
		w := h * previewWidth / previewHeight
		x, y := r.X+(r.W-w)/2, r.Y+(r.H-h)/2
		shadowW := w * 0.52
		p.Fill(ui.Rect{X: r.X + (r.W-shadowW)/2, Y: y + h*0.92, W: shadowW, H: max(3, h*0.038)},
			ui.Hex("#071222").Alpha(0.56), 9)
		p.Image(model, ui.Rect{X: x, Y: y, W: w, H: h}, ui.Contain)
	})
}

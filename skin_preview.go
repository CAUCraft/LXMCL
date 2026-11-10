package main

import (
	"bytes"
	_ "embed"
	"image"
	"image/draw"
	"image/png"

	"github.com/egoist/mygo/ui"
)

//go:embed ui/assets/luanxiao.png
var clubSkinPNG []byte

type skinPreview struct {
	head, front, back *ui.Bitmap
}

var clubSkin = func() skinPreview {
	img, err := png.Decode(bytes.NewReader(clubSkinPNG))
	if err != nil {
		panic(err)
	}
	return renderSkin(img, false)
}()

// Compose the base and outer layers of the Java skin atlas into pixel art.
func renderSkin(img image.Image, slim bool) skinPreview {
	return skinPreview{
		head:  ui.NewBitmap(skinHead(img)).Pixelated(),
		front: ui.NewBitmap(skinSide(img, slim, false)).Pixelated(),
		back:  ui.NewBitmap(skinSide(img, slim, true)).Pixelated(),
	}
}

func skinHead(img image.Image) *image.RGBA {
	head := image.NewRGBA(image.Rect(0, 0, 8, 8))
	draw.Draw(head, head.Bounds(), img, image.Pt(8, 8), draw.Src)
	draw.Draw(head, head.Bounds(), img, image.Pt(40, 8), draw.Over)
	return head
}

func skinSide(img image.Image, slim, back bool) *image.RGBA {
	arm := 4
	if slim {
		arm = 3
	}
	out := image.NewRGBA(image.Rect(0, 0, 16, 32))
	legacy := img.Bounds().Dy() == 32
	face := func(x, y, w, h, u, v, layerU, layerV int, flip bool) {
		if !flip {
			box := image.Rect(x, y, x+w, y+h)
			draw.Draw(out, box, img, image.Pt(u, v), draw.Src)
			if layerV < img.Bounds().Dy() {
				draw.Draw(out, box, img, image.Pt(layerU, layerV), draw.Over)
			}
			return
		}
		for j := range h {
			for i := range w {
				out.Set(x+i, y+j, img.At(u+w-1-i, v+j))
			}
		}
	}
	head, body, limb, sleeve := 8, 20, 4, 4
	if back {
		head, body, limb, sleeve = 24, 32, 12, 8+arm
	}
	face(4, 0, 8, 8, head, 8, head+32, 8, false)
	face(4, 8, 8, 12, body, 20, body, 36, false)
	rightArm, leftArm, rightLeg, leftLeg := 4-arm, 12, 4, 8
	if back {
		rightArm, leftArm, rightLeg, leftLeg = leftArm, rightArm, leftLeg, rightLeg
	}
	face(rightArm, 8, arm, 12, 40+sleeve, 20, 40+sleeve, 36, false)
	face(rightLeg, 20, 4, 12, limb, 20, limb, 36, false)
	if legacy {
		face(leftArm, 8, arm, 12, 40+sleeve, 20, 0, 64, true)
		face(leftLeg, 20, 4, 12, limb, 20, 0, 64, true)
	} else {
		face(leftArm, 8, arm, 12, 32+sleeve, 52, 48+sleeve, 52, false)
		face(leftLeg, 20, 4, 12, 16+limb, 52, limb, 52, false)
	}
	return out
}

func renderCape(img image.Image) *ui.Bitmap {
	b := img.Bounds()
	scale := 1
	if b.Dx()%64 == 0 && b.Dx()/64 == b.Dy()/32 && b.Dy()%32 == 0 {
		scale = b.Dx() / 64
	}
	out := image.NewRGBA(image.Rect(0, 0, 10*scale, 16*scale))
	draw.Draw(out, out.Bounds(), img, image.Pt(1*scale, 1*scale), draw.Src)
	return ui.NewBitmap(out).Pixelated()
}

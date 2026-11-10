package main

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"sort"

	"github.com/egoist/mygo/ui"
)

//go:embed ui/assets/luanxiao.png
var clubSkinPNG []byte

type skinPreview struct {
	head  *ui.Bitmap
	atlas *image.NRGBA
}

var clubSkin = func() skinPreview {
	img, err := png.Decode(bytes.NewReader(clubSkinPNG))
	if err != nil {
		panic(err)
	}
	return renderSkin(img)
}()

func renderSkin(img image.Image) skinPreview {
	return skinPreview{head: ui.NewBitmap(skinHead(img)).Pixelated(), atlas: textureAtlas(img)}
}

func textureAtlas(img image.Image) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, img.Bounds().Dx(), img.Bounds().Dy()))
	draw.Draw(dst, dst.Bounds(), img, img.Bounds().Min, draw.Src)
	return dst
}

func skinHead(img image.Image) *image.RGBA {
	head := image.NewRGBA(image.Rect(0, 0, 8, 8))
	draw.Draw(head, head.Bounds(), img, image.Pt(8, 8), draw.Src)
	draw.Draw(head, head.Bounds(), img, image.Pt(40, 8), draw.Over)
	return head
}

// Placeholder for a profile whose default skin has no texture in its API response.
var placeholderSkin = func() skinPreview {
	img := image.NewNRGBA(image.Rect(0, 0, 64, 32))
	for _, part := range []struct {
		area image.Rectangle
		ink  color.NRGBA
	}{
		{image.Rect(0, 0, 32, 16), color.NRGBA{142, 175, 210, 255}},
		{image.Rect(16, 16, 40, 32), color.NRGBA{75, 111, 153, 255}},
		{image.Rect(40, 16, 56, 32), color.NRGBA{106, 145, 185, 255}},
		{image.Rect(0, 16, 16, 32), color.NRGBA{61, 86, 122, 255}},
	} {
		draw.Draw(img, part.area, image.NewUniform(part.ink), image.Point{}, draw.Src)
	}
	return renderSkin(img)
}()

type skinPoint struct{ x, y, z float64 }

func (p skinPoint) minus(q skinPoint) skinPoint {
	return skinPoint{p.x - q.x, p.y - q.y, p.z - q.z}
}

type skinBox struct {
	x, y, z, grow float64
	w, h, d, u, v int
	outer, mirror bool
}

type skinFace struct {
	p, du, dv skinPoint
	texture   *image.NRGBA
	uv        image.Rectangle
	light     float64
	flipU     bool
	opaque    bool
}

// Java skin and cape textures use the same six-face cuboid unwrap. The
// vertices follow the texture's U and V axes as seen from outside each face.
func (b skinBox) faces(texture *image.NRGBA, cape bool) []skinFace {
	w, h, d, u, v := b.w, b.h, b.d, b.u, b.v
	x0, y0, z0 := b.x-b.grow, b.y-b.grow, b.z-b.grow
	x1, y1, z1 := b.x+float64(w)+b.grow, b.y+float64(h)+b.grow, b.z+float64(d)+b.grow
	ltb, rtb := skinPoint{x0, y0, z0}, skinPoint{x1, y0, z0}
	ltf, rtf := skinPoint{x0, y0, z1}, skinPoint{x1, y0, z1}
	lbb, rbb := skinPoint{x0, y1, z0}, skinPoint{x1, y1, z0}
	lbf, rbf := skinPoint{x0, y1, z1}, skinPoint{x1, y1, z1}
	quads := [6][3]skinPoint{
		{ltb, rtb, ltf}, {lbf, rbf, lbb}, {ltb, ltf, lbb},
		{ltf, rtf, lbf}, {rtf, rtb, rbf}, {rtb, ltb, rbb},
	}
	rects := [6]image.Rectangle{
		atlasRect(u+d, v, w, d), atlasRect(u+d+w, v, w, d),
		atlasRect(u, v+d, d, h), atlasRect(u+d, v+d, w, h),
		atlasRect(u+d+w, v+d, d, h), atlasRect(u+2*d+w, v+d, w, h),
	}
	if b.mirror {
		rects[2], rects[4] = rects[4], rects[2]
	}
	scale := 1
	if cape && texture.Bounds().Dx()%64 == 0 && texture.Bounds().Dy()*2 == texture.Bounds().Dx() {
		scale = texture.Bounds().Dx() / 64
	}
	faces := make([]skinFace, 0, len(quads))
	for i, q := range quads {
		if cape {
			for j, p := range q {
				// Turn the cape's outside away from the torso and hang it
				// from the shoulders at a slight angle.
				q[j] = skinPoint{-p.x, 8 + p.y*0.966 - p.z*0.259, -2.3 - p.y*0.259 - p.z*0.966}
			}
		}
		r := rects[i]
		uv := image.Rectangle{Min: r.Min.Mul(scale), Max: r.Max.Mul(scale)}
		if uv.In(texture.Bounds()) {
			faces = append(faces, skinFace{
				p: q[0], du: q[1].minus(q[0]), dv: q[2].minus(q[0]),
				texture: texture, uv: uv, flipU: b.mirror, opaque: !b.outer && !cape,
			})
		}
	}
	return faces
}

func atlasRect(u, v, w, h int) image.Rectangle {
	return image.Rect(u, v, u+w, v+h)
}

func skinModel(atlas, cape *image.NRGBA, slim bool) []skinFace {
	legacy := atlas.Bounds().Dy() == 32
	arm := 4
	if slim && !legacy {
		arm = 3
	}
	var faces []skinFace
	add := func(x, y, z float64, w, h, d, u, v, outerU, outerV int, mirror bool, grow float64) {
		box := skinBox{x: x, y: y, z: z, w: w, h: h, d: d, u: u, v: v, mirror: mirror}
		faces = append(faces, box.faces(atlas, false)...)
		if !legacy || y == 0 {
			box.u, box.v, box.grow, box.outer = outerU, outerV, grow, true
			faces = append(faces, box.faces(atlas, false)...)
		}
	}
	add(-4, 0, -4, 8, 8, 8, 0, 0, 32, 0, false, 0.5)
	add(-4, 8, -2, 8, 12, 4, 16, 16, 16, 32, false, 0.25)
	add(float64(-4-arm), 8.25, -2, arm, 12, 4, 40, 16, 40, 32, false, 0.25)
	add(-4, 20, -2, 4, 12, 4, 0, 16, 0, 32, false, 0.25)
	if legacy {
		add(4, 8.25, -2, arm, 12, 4, 40, 16, 0, 0, true, 0)
		add(0, 20, -2, 4, 12, 4, 0, 16, 0, 0, true, 0)
	} else {
		add(4, 8.25, -2, arm, 12, 4, 32, 48, 48, 48, false, 0.25)
		add(0, 20, -2, 4, 12, 4, 16, 48, 0, 48, false, 0.25)
	}
	if cape != nil {
		faces = append(faces, (skinBox{x: -5, w: 10, h: 16, d: 1}).faces(cape, true)...)
	}
	return faces
}

const previewWidth, previewHeight = 240, 304

// Rotate an orthographic camera about the model. A depth buffer keeps the
// limbs, jacket and cape in order even when their faces cross in the image.
func drawSkinModel(dst *image.RGBA, depth []float64, faces []skinFace, yaw, pitch float64) {
	clear(dst.Pix)
	for i := range depth {
		depth[i] = math.Inf(-1)
	}
	sy, cy := math.Sincos(yaw)
	sp, cp := math.Sincos(pitch)
	rotate := func(p skinPoint) skinPoint {
		z := p.z*cy - p.x*sy
		return skinPoint{p.x*cy + p.z*sy, p.y*cp - z*sp, p.y*sp + z*cp}
	}
	visible := make([]skinFace, 0, len(faces))
	scale := float64(dst.Bounds().Dy()) / 39
	for _, f := range faces {
		f.p.y -= 16
		f.p, f.du, f.dv = rotate(f.p), rotate(f.du), rotate(f.dv)
		u, v := f.du, f.dv
		n := skinPoint{u.y*v.z - u.z*v.y, u.z*v.x - u.x*v.z, u.x*v.y - u.y*v.x}
		if n.z <= 0.01 {
			continue
		}
		lenN := math.Sqrt(n.x*n.x + n.y*n.y + n.z*n.z)
		f.light = 0.67 + 0.33*max(0, (-0.36*n.x-0.64*n.y+0.68*n.z)/lenN)
		f.p.x = f.p.x*scale + float64(dst.Bounds().Dx())/2
		f.p.y = f.p.y*scale + float64(dst.Bounds().Dy())/2
		f.du.x, f.du.y, f.dv.x, f.dv.y = u.x*scale, u.y*scale, v.x*scale, v.y*scale
		visible = append(visible, f)
		rasterSkinFace(dst, depth, f, false)
	}
	// Handle partially transparent overlays after all opaque texels, from back to front.
	centerZ := func(f skinFace) float64 { return f.p.z + (f.du.z+f.dv.z)/2 }
	sort.SliceStable(visible, func(i, j int) bool { return centerZ(visible[i]) < centerZ(visible[j]) })
	for _, f := range visible {
		if !f.opaque {
			rasterSkinFace(dst, depth, f, true)
		}
	}
}

func rasterSkinFace(dst *image.RGBA, depth []float64, f skinFace, translucent bool) {
	p, du, dv := f.p, f.du, f.dv
	det := du.x*dv.y - du.y*dv.x
	w, h := dst.Bounds().Dx(), dst.Bounds().Dy()
	minX := max(0, int(math.Floor(p.x+min(0, du.x)+min(0, dv.x))))
	minY := max(0, int(math.Floor(p.y+min(0, du.y)+min(0, dv.y))))
	maxX := min(w, int(math.Ceil(p.x+max(0, du.x)+max(0, dv.x))))
	maxY := min(h, int(math.Ceil(p.y+max(0, du.y)+max(0, dv.y))))
	for y := minY; y < maxY; y++ {
		dy := float64(y) + 0.5 - p.y
		for x := minX; x < maxX; x++ {
			dx := float64(x) + 0.5 - p.x
			u, v := (dx*dv.y-dy*dv.x)/det, (du.x*dy-du.y*dx)/det
			if u < 0 || u >= 1 || v < 0 || v >= 1 {
				continue
			}
			i := y*w + x
			z := p.z + du.z*u + dv.z*v
			if z <= depth[i] {
				continue
			}
			tx, ty := int(u*float64(f.uv.Dx())), int(v*float64(f.uv.Dy()))
			if f.flipU {
				tx = f.uv.Dx() - 1 - tx
			}
			src := f.texture.NRGBAAt(f.uv.Min.X+tx, f.uv.Min.Y+ty)
			if f.opaque {
				src.A = 255
			}
			if src.A == 0 || translucent == (src.A == 255) {
				continue
			}
			r, g, b := uint8(float64(src.R)*f.light), uint8(float64(src.G)*f.light), uint8(float64(src.B)*f.light)
			pix := dst.Pix[i*4 : i*4+4]
			if translucent {
				a, keep := uint32(src.A), uint32(255-src.A)
				pix[0] = uint8((uint32(r)*a + uint32(pix[0])*keep) / 255)
				pix[1] = uint8((uint32(g)*a + uint32(pix[1])*keep) / 255)
				pix[2] = uint8((uint32(b)*a + uint32(pix[2])*keep) / 255)
				pix[3] = src.A + uint8(uint32(pix[3])*keep/255)
			} else {
				depth[i] = z
				pix[0], pix[1], pix[2], pix[3] = r, g, b, 255
			}
		}
	}
}

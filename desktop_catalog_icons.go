package main

import (
	"time"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/display"
)

type catalogIcon struct {
	image   ui.ImageSource
	pending bool
	retry   time.Time
}

func (a *desktop) projectIcon(c *ui.Context, key, raw string, fallback *ui.SVG, size float32) {
	if img := a.catalogIcon(raw); img != nil {
		display.Avatar(c.Key(key), "", display.AvatarProps{Size: size, Image: img, Shape: display.AvatarSquare})
		return
	}
	leadIcon(c.Key(key), fallback, size)
}

func (a *desktop) projectHeading(c *ui.Context, key, title, raw string, glyph *ui.SVG, controls func()) {
	ui.Row(c).Gap(10).AlignItems(ui.Center).Shrink(0).Children(func() {
		a.projectIcon(c, key, raw, glyph, 36)
		display.Text(c, title, display.TextProps{Strong: true, FontSize: 19, Rows: 1}).Grow(1).MinWidth(0).Ellipsis("…")
		controls()
	})
}

func (a *desktop) catalogIcon(raw string) ui.ImageSource {
	if raw == "" || a.ctx == nil || len(raw) > 2048 {
		return nil
	}
	if a.catalogIcons == nil {
		a.catalogIcons = make(map[string]catalogIcon)
		a.catalogIconSlots = make(chan struct{}, 6)
	}
	if entry, ok := a.catalogIcons[raw]; ok {
		if entry.image != nil || entry.pending || time.Now().Before(entry.retry) {
			return entry.image
		}
	}
	if len(a.catalogIcons) >= 192 {
		for key, entry := range a.catalogIcons {
			if !entry.pending {
				delete(a.catalogIcons, key)
				break
			}
		}
		if len(a.catalogIcons) >= 192 {
			return nil
		}
	}
	a.catalogIcons[raw] = catalogIcon{pending: true}
	go func() {
		select {
		case <-a.ctx.Done():
			return
		case a.catalogIconSlots <- struct{}{}:
		}
		defer func() { <-a.catalogIconSlots }()
		var source ui.ImageSource
		if img, err := launcher.LoadCatalogIcon(a.ctx, raw); err == nil {
			source = ui.NewBitmap(img)
		}
		a.post(func() {
			a.catalogIcons[raw] = catalogIcon{image: source, retry: time.Now().Add(time.Minute)}
		})
	}()
	return nil
}

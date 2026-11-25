package main

import (
	"cmp"
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/layout"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
)

type modReleaseNotesView struct {
	target              launcher.Instance
	request             uint64
	file, digest, title string
	version             launcher.PackVersion
	cancel              context.CancelFunc
	done                <-chan modReleaseNotesResult
	text                string
	failed              bool
}

type modReleaseNotesResult struct {
	changelog json.RawMessage
	text      string
	failed    bool
}

func (a *desktop) modReleaseNotesTarget(p *modReleaseNotesView) bool {
	v := &a.content
	if a.page != pageLibrary || a.inspected != p.target.ID || a.detailPage != detailContent ||
		v.id != p.target.ID || v.revision != p.target.Revision || v.kind() != launcher.Mods || a.contentRequest != p.request {
		return false
	}
	i, exists := a.state.Find(p.target.ID)
	m := v.mods[p.file]
	return exists && i == p.target && m.Digest == p.digest &&
		m.Update.ProjectID == p.version.ProjectID && m.Update.ID == p.version.ID
}

func (a *desktop) closeModReleaseNotes() {
	if p := a.content.modNotes; p != nil {
		if p.cancel != nil {
			p.cancel()
		}
		a.content.modNotes = nil
	}
}

func (a *desktop) discardStaleModReleaseNotes() {
	if p := a.content.modNotes; p != nil && !a.modReleaseNotesTarget(p) {
		a.closeModReleaseNotes()
	}
}

func (a *desktop) openModReleaseNotes(c *ui.Context, i launcher.Instance, file string, m launcher.InstalledMod) {
	a.closeModReleaseNotes()
	p := &modReleaseNotesView{target: i, request: a.contentRequest, file: file, digest: m.Digest,
		title: cmp.Or(m.Title, m.File.Name, file), version: m.Update}
	a.content.modNotes = p
	if text, known := p.version.ChangelogText(); known {
		p.text = modReleaseNotesText(text)
		return
	}
	a.loadModReleaseNotes(c, p)
}

func (a *desktop) loadModReleaseNotes(c *ui.Context, p *modReleaseNotesView) {
	if a.ctx == nil || p.done != nil || !a.modReleaseNotesTarget(p) {
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	done := make(chan modReleaseNotesResult, 1)
	p.cancel, p.done, p.failed = cancel, done, false
	project, version := p.version.ProjectID, p.version.ID
	game, services := a.game, c.Services()
	go func() {
		defer cancel()
		detail, err := game.ModrinthVersion(ctx, project, version)
		if err != nil {
			done <- modReleaseNotesResult{failed: true}
		} else {
			if len(detail.Changelog) == 0 {
				detail.Changelog = json.RawMessage("null")
			}
			text, known := detail.ChangelogText()
			done <- modReleaseNotesResult{changelog: detail.Changelog, text: modReleaseNotesText(text), failed: !known}
		}
		services.Invalidate()
	}()
}

func modReleaseNotesText(text string) string {
	text = strings.TrimSpace(text)
	const maxBytes = 24 << 10
	if len(text) <= maxBytes {
		return text
	}
	end := maxBytes
	for !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end] + "\n\n…（发布说明过长，已截断）"
}

func (a *desktop) modReleaseNotes(c *ui.Context, p *modReleaseNotesView) {
	select {
	case result := <-p.done:
		p.done, p.cancel, p.text, p.failed = nil, nil, result.text, result.failed
		if !result.failed {
			// Keep notes with the checked candidate until the inventory is refreshed.
			m := a.content.mods[p.file]
			m.Update.Changelog = result.changelog
			a.content.mods[p.file] = m
		}
	default:
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		if icon(c, lucide.ArrowLeft(), "返回模组列表", false) {
			a.closeModReleaseNotes()
		}
		display.Text(c, "发布说明", display.TextProps{Strong: true, FontSize: 16}).Grow(1)
		if p.failed && button(c, "重试", action.Default, a.ctx == nil) {
			a.loadModReleaseNotes(c, p)
		}
	})
	display.Text(c, p.title, display.TextProps{Strong: true, Rows: 1}).Ellipsis("…").Tooltip(p.title)
	label := "Modrinth · 更新候选 " + cmp.Or(p.version.Number, p.version.ID)
	muted(c, label).SingleLine().Ellipsis("…").Tooltip(label)
	switch {
	case p.done != nil:
		flightLoading(c, "正在读取发布说明…")
	case p.failed:
		feedback.Alert(c, "无法读取发布说明", feedback.AlertProps{Type: feedback.Error})
	case p.text == "":
		muted(c, "此版本没有发布说明。")
	default:
		layout.ScrollArea(c.Key(p), layout.ScrollAreaProps{}, func() {
			display.Text(c, p.text, display.TextProps{}).Selectable()
		}).Grow(1).MinHeight(0)
	}
}

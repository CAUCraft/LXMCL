package main

import (
	"errors"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo"
)

type modRevealRequest struct {
	listing *resourceListing
}

func (a *desktop) inLocalModList() bool {
	v := &a.content
	return a.page == pageLibrary && a.detailPage == detailContent && a.inspected == v.id &&
		v.kind() == launcher.Mods && !v.reading && !a.form.open && !a.busy &&
		v.modNotes == nil && v.modPreview == nil && v.restoreMods == nil && v.comparison == nil && v.copyLocal == nil
}

func (a *desktop) revealModFile(i launcher.Instance, file launcher.ContentFile) {
	v := &a.content
	listing := v.listing
	if a.ctx == nil || listing == nil || v.reveal != nil && v.reveal.listing == listing || !a.inLocalModList() {
		return
	}
	pending := &modRevealRequest{listing: listing}
	v.reveal = pending
	request := a.contentRequest
	go func() {
		path, err := file.ModRevealPath(i)
		a.post(func() {
			if v.reveal != pending {
				return
			}
			v.reveal = nil
			current, ok := a.state.Find(i.ID)
			if !ok || current != i || a.inspected != i.ID || request != a.contentRequest ||
				!a.inLocalModList() || v.listing != listing ||
				v.query != listing.query || v.filter != listing.filter || v.modFilter != listing.modFilter {
				return
			}
			if err != nil {
				a.message("无法定位模组文件", errors.New("请刷新资源列表后重试。"))
				return
			}
			mygo.Shell.ShowItemInFolder(path)
		})
	}()
}

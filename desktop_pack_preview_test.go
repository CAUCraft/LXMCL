package main

import (
	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
	"testing"
	"time"
)

func TestResourcePackDirectoryStaysReadOnly(t *testing.T) {
	a, tt := testDesktop(t)
	i := a.state.Instances[1]
	a.inspect(i)
	a.detailPage = detailContent
	files := []launcher.ContentFile{{Name: "Spring", Enabled: true, Directory: true}, {Name: "Winter.zip", Enabled: false}}
	a.content = contentView{id: i.ID, revision: i.Revision, files: files}
	tt.SetSize(820, 530)
	tt.Frame()
	if err := tt.Click("全选当前列表"); err != nil {
		t.Fatal(err)
	}
	if len(a.content.selected) != 1 {
		t.Fatal("directory included in bulk actions", a.content.selected)
	}
	for _, f := range files {
		if _, ok := tt.Find("预览 " + f.DiskName()); !ok {
			t.Fatal(tt.Texts())
		}
	}
	p := &resourcePackView{file: files[0]}
	p.data.Description = "初春的森林"
	p.data.Format = "34"
	a.content.packPreview = p
	for _, dark := range []bool{false, true} {
		tt.SetDark(dark)
		tt.Advance(250 * time.Millisecond)
		tt.Frame()
		if !tt.HasText("无封面") || !tt.HasText("初春的森林") || !tt.HasText("格式编号 · 34") {
			t.Fatal(tt.Texts())
		}
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.content.packPreview != nil || len(a.content.selected) != 1 || a.state.Selected != 1 || a.detailPage != detailContent {
		t.Fatal("back changed the resource list or launch selection")
	}
}

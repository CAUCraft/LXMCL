package main

import (
	"errors"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/saves"
)

func TestWorldSavedPackClipboard(t *testing.T) {
	const title = "游戏上次保存的数据包清单"
	data := saves.WorldDataPacks{
		Saved: true,
		Local: []saves.DataPack{{Name: "never-copy-local.zip"}},
		Enabled: []saves.DataPack{
			{Name: "file/河谷.zip"}, {Name: "vanilla"}, {Name: "file/河谷.zip"},
			{Name: "file//home/player/private.zip"}, {Name: `C:\Users\player\private.zip`},
			{Name: `\\server\share\private.zip`}, {Name: "file/notes\u2028next"}, {Name: "file/notes\u202Enext"},
		},
		Disabled: []saves.DataPack{{Name: "file/河谷.zip"}, {Name: "custom-pack"}, {Name: "file/河谷.zip"}},
	}
	want := title + "\n\n上次启用\n1. 河谷.zip（文件）\n2. vanilla\n3. 河谷.zip（文件）" +
		"\n4. 标识已隐藏\n5. 标识已隐藏\n6. 标识已隐藏\n7. 标识已隐藏\n8. 标识已隐藏" +
		"\n\n上次停用\n1. 河谷.zip（文件）\n2. custom-pack\n3. 河谷.zip（文件）"
	if got := worldSavedPackClipboard(data); got != want {
		t.Fatalf("saved lists = %q, want %q", got, want)
	}

	for _, tc := range []struct {
		label string
		data  saves.WorldDataPacks
		want  string
	}{
		{"absent", saves.WorldDataPacks{}, ""},
		{"failed", saves.WorldDataPacks{Saved: true, SavedErr: errors.New("read failed")}, ""},
		{"empty and unknown", saves.WorldDataPacks{Saved: true, Enabled: []saves.DataPack{}},
			title + "\n\n上次启用\n（空）\n\n上次停用\n未知（没有保存记录）"},
		{"both empty", saves.WorldDataPacks{Saved: true, Enabled: []saves.DataPack{}, Disabled: []saves.DataPack{}},
			title + "\n\n上次启用\n（空）\n\n上次停用\n（空）"},
	} {
		t.Run(tc.label, func(t *testing.T) {
			if got := worldSavedPackClipboard(tc.data); got != tc.want {
				t.Fatalf("saved lists = %q, want %q", got, tc.want)
			}
		})
	}
}

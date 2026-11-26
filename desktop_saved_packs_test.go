package main

import (
	"testing"

	"github.com/CAUCraft/LXMCL/internal/content"
)

func TestSavedPackOrderClipboard(t *testing.T) {
	rows := []content.SavedPack{
		{ID: "file/河谷.zip", Kind: content.LocalPack, File: &content.File{Name: "河谷.zip"}},
		{ID: "vanilla", Kind: content.BuiltinPack},
		{ID: "file/vanilla", Kind: content.LocalPack, File: &content.File{Name: "vanilla", Directory: true}},
		{ID: "file/河谷.zip", Kind: content.MissingPack},
		{ID: "旧包.zip", Kind: content.MissingPack},
		{ID: "Fabric Mods", Kind: content.LoaderPackID},
		{ID: "custom-pack", Kind: content.OtherPackID},
		{ID: "file/../private.zip", Kind: content.MissingPack},
		{ID: "/home/player/private.zip", Kind: content.OtherPackID},
		{ID: `C:\Users\player\private.zip`, Kind: content.OtherPackID},
		{ID: "C:private.zip", Kind: content.OtherPackID},
		{ID: "file/notes\u2028more", Kind: content.MissingPack},
	}
	const want = "游戏上次保存的资源包顺序\n1. 河谷.zip（文件）\n2. vanilla\n3. vanilla（文件）\n4. 河谷.zip（文件）\n5. 旧包.zip\n6. Fabric Mods\n7. custom-pack\n8. 标识已隐藏\n9. 标识已隐藏\n10. 标识已隐藏\n11. 标识已隐藏\n12. 标识已隐藏"
	if got := savedPackOrderClipboard(rows); got != want {
		t.Fatalf("saved order = %q, want %q", got, want)
	}
	if got := savedPackOrderClipboard(nil); got != "" {
		t.Fatalf("empty saved order = %q", got)
	}
}

package main

import (
	"testing"

	"github.com/CAUCraft/LXMCL/internal/content"
)

func TestPackComparisonReport(t *testing.T) {
	const header = "资源包比对清单\n左侧 · 当前实例，右侧 · 对照实例\n仅比较名称、类型和文件停用状态。未校验包内容。"
	present, absent, disabled, duplicate := content.PackPresent, content.PackAbsent, content.PackDisabled, content.PackDuplicate
	rows := []content.ResourcePackMatch{
		{Name: "01 重复.zip", Current: duplicate, Other: duplicate},
		{Name: "02 河谷.zip", Directory: true, Current: present, Other: present},
		{Name: "02 河谷.zip", Current: present, Other: disabled},
		{Name: "03 侧景.zip", Current: absent, Other: disabled},
		{Name: "04 屋顶.zip", Current: present, Other: absent},
		{Name: "05 烟花.zip", Current: disabled, Other: absent},
		{Name: "/home/private.zip", Current: present, Other: absent},
		{Name: `C:\Users\private.zip`, Current: present, Other: absent},
		{Name: "C:private.zip", Current: present, Other: absent},
		{Name: "换行\n仅右侧.zip", Current: present, Other: absent},
		{Name: "分隔\u2028隐藏", Directory: true, Current: present, Other: absent},
		{Name: "双向\u202e文件.zip", Current: present, Other: absent},
	}
	want := header + "\n\n仅左侧\n" +
		"- 04 屋顶.zip · ZIP · 左侧 未停用 · 右侧 缺少\n" +
		"- 05 烟花.zip · ZIP · 左侧 已停用 · 右侧 缺少\n" +
		"- 名称已隐藏 · ZIP · 左侧 未停用 · 右侧 缺少\n" +
		"- 名称已隐藏 · ZIP · 左侧 未停用 · 右侧 缺少\n" +
		"- 名称已隐藏 · ZIP · 左侧 未停用 · 右侧 缺少\n" +
		"- 名称已隐藏 · ZIP · 左侧 未停用 · 右侧 缺少\n" +
		"- 名称已隐藏 · 目录 · 左侧 有目录 · 右侧 缺少\n" +
		"- 名称已隐藏 · ZIP · 左侧 未停用 · 右侧 缺少\n\n" +
		"仅右侧\n- 03 侧景.zip · ZIP · 左侧 缺少 · 右侧 已停用\n\n" +
		"停用状态不同\n- 02 河谷.zip · ZIP · 左侧 未停用 · 右侧 已停用\n\n" +
		"两侧共有\n- 02 河谷.zip · 目录 · 左侧 有目录 · 右侧 有目录\n\n" +
		"同名重复（状态待确认）\n- 01 重复.zip · ZIP · 左侧 同名重复 · 右侧 同名重复"
	if got := packComparisonReport(rows); got != want {
		t.Fatalf("comparison report = %q, want %q", got, want)
	}
	if got, want := packComparisonReport(nil), header+"\n\n两侧均无本地资源包。"; got != want {
		t.Fatalf("empty report = %q, want %q", got, want)
	}
}

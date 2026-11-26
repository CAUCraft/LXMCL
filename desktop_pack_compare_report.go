package main

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

type packMatchGroup int

const (
	packOnlyCurrent packMatchGroup = iota
	packOnlyPeer
	packStateChanged
	packShared
	packRepeated
)

var packMatchGroups = [...]struct{ label, heading string }{
	packOnlyCurrent:  {"仅当前", "仅左侧"},
	packOnlyPeer:     {"仅对照", "仅右侧"},
	packStateChanged: {"停用状态不同", "停用状态不同"},
	packShared:       {"两侧共有", "两侧共有"},
	packRepeated:     {"同名重复", "同名重复（状态待确认）"},
}

func packGroup(p content.ResourcePackMatch) packMatchGroup {
	switch {
	case p.Current == content.PackDuplicate || p.Other == content.PackDuplicate:
		return packRepeated
	case p.Other == content.PackAbsent:
		return packOnlyCurrent
	case p.Current == content.PackAbsent:
		return packOnlyPeer
	case p.Current != p.Other:
		return packStateChanged
	default:
		return packShared
	}
}

func packComparisonKind(directory bool) string {
	if directory {
		return "目录"
	}
	return "ZIP"
}

func packComparisonReport(rows []content.ResourcePackMatch) string {
	const header = "资源包比对清单\n左侧 · 当前实例，右侧 · 对照实例\n仅比较名称、类型和文件停用状态。未校验包内容。"
	if len(rows) == 0 {
		return header + "\n\n两侧均无本地资源包。"
	}
	var groups [len(packMatchGroups)][]content.ResourcePackMatch
	for _, row := range rows {
		group := packGroup(row)
		groups[group] = append(groups[group], row)
	}
	var text strings.Builder
	text.WriteString(header)
	for group, matches := range groups {
		if len(matches) == 0 {
			continue
		}
		text.WriteString("\n\n" + packMatchGroups[group].heading)
		for _, row := range matches {
			name := row.Name
			if !storage.ValidFilename(name) || strings.ContainsAny(name, "\u2028\u2029") ||
				strings.ContainsFunc(name, func(r rune) bool { return unicode.Is(unicode.Bidi_Control, r) }) {
				name = "名称已隐藏"
			}
			fmt.Fprintf(&text, "\n- %s · %s · 左侧 %s · 右侧 %s", name, packComparisonKind(row.Directory),
				packComparisonSide(row.Current, row.Directory), packComparisonSide(row.Other, row.Directory))
		}
	}
	return text.String()
}

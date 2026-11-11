package storage

import (
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
)

// ValidPortablePath reports whether a slash-separated relative path can be used
// in managed content on Windows as well as Unix.
func ValidPortablePath(name string) bool {
	if name == "." || len(name) > 1024 || !fs.ValidPath(name) || !filepath.IsLocal(filepath.FromSlash(name)) ||
		strings.ContainsAny(name, "\\:\"<>|?*") || strings.ContainsFunc(name, unicode.IsControl) {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if len(part) > 255 || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return false
		}
		stem, _, _ := strings.Cut(part, ".")
		stem = strings.ToUpper(strings.TrimRight(stem, " "))
		switch stem {
		case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
			return false
		}
		for _, prefix := range []string{"COM", "LPT"} {
			if suffix, ok := strings.CutPrefix(stem, prefix); ok && slices.Contains([]string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "¹", "²", "³"}, suffix) {
				return false
			}
		}
	}
	return true
}

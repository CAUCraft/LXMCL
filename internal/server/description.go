package server

import (
	"regexp"
	"strings"
	"unicode"
)

// Server descriptions may be plain strings, component arrays or styled text
// with nested extras. Keep line breaks and visible text for the native widget.
func descriptionText(value any) string {
	var text strings.Builder
	var appendText func(any, int)
	appendText = func(value any, depth int) {
		if depth > 32 {
			return
		}
		switch v := value.(type) {
		case string:
			text.WriteString(v)
		case []any:
			for _, item := range v {
				appendText(item, depth+1)
			}
		case map[string]any:
			if s, ok := v["text"].(string); ok {
				text.WriteString(s)
			}
			appendText(v["extra"], depth+1)
		}
	}
	appendText(value, 0)
	return plainStatusText(text.String(), 256)
}

var formattingCode = regexp.MustCompile(`(?i)§[0-9a-fk-orx]`)

func plainStatusText(s string, limit int) string {
	s = formattingCode.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.Map(func(r rune) rune {
		switch r {
		case '\r':
			return '\n'
		case '\t':
			return ' '
		case '\n':
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	runes := []rune(strings.TrimSpace(s))
	if len(runes) > limit {
		return string(runes[:limit-1]) + "…"
	}
	return string(runes)
}

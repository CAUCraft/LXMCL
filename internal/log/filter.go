package log

import (
	"regexp"
	"strings"
)

var problemLine = regexp.MustCompile(`(?im)(\b(?:WARN(?:ING)?|ERROR|FATAL)\b|(?:Error|Exception)\b|^Caused by:)`)

// Filter keeps complete log entries or Minecraft crash report sections so
// matching a stack frame also keeps its surrounding diagnostic context.
func Filter(text, query string, problemsOnly bool) string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" && !problemsOnly {
		return text
	}
	crashReport := strings.HasPrefix(strings.TrimPrefix(text, "\uFEFF"), "---- Minecraft Crash Report ----")
	var result, block strings.Builder
	flush := func() {
		entry := block.String()
		if strings.Contains(strings.ToLower(entry), query) && (!problemsOnly || problemLine.MatchString(entry)) {
			result.WriteString(entry)
		}
		block.Reset()
	}
	for _, line := range strings.SplitAfter(text, "\n") {
		boundary := strings.HasPrefix(line, "[")
		if crashReport {
			boundary = strings.HasPrefix(line, "-- ") && strings.HasSuffix(strings.TrimRight(line, "\r\n"), " --")
		}
		if boundary {
			flush()
		}
		block.WriteString(line)
	}
	flush()
	return result.String()
}

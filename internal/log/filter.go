package log

import (
	"regexp"
	"strings"
)

var problemLine = regexp.MustCompile(`(?im)(\b(?:WARN(?:ING)?|ERROR|FATAL)\b|(?:Error|Exception)\b|^Caused by:)`)

// Filter keeps complete bracket-prefixed log entries, including their following
// stack traces. Query and severity narrow the preview without losing context.
func Filter(text, query string, problemsOnly bool) string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" && !problemsOnly {
		return text
	}
	var result, block strings.Builder
	flush := func() {
		entry := block.String()
		if strings.Contains(strings.ToLower(entry), query) && (!problemsOnly || problemLine.MatchString(entry)) {
			result.WriteString(entry)
		}
		block.Reset()
	}
	for _, line := range strings.SplitAfter(text, "\n") {
		if strings.HasPrefix(line, "[") {
			flush()
		}
		block.WriteString(line)
	}
	flush()
	return result.String()
}

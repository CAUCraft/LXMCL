package log

import "strings"

type Diagnosis struct{ Cause, Evidence string }

// Diagnose prefers a specific cause over a loader's general incompatibility heading.
func Diagnose(log string) Diagnosis {
	var fallback Diagnosis
	for line := range strings.SplitSeq(log, "\n") {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		cause := ""
		switch {
		case strings.Contains(line, "UnsupportedClassVersionError"):
			cause = "java"
		case strings.Contains(line, "OutOfMemoryError: Java heap space") || strings.Contains(line, "OutOfMemoryError: GC overhead limit exceeded"):
			cause = "heap"
		case strings.Contains(lower, "requires") && (strings.Contains(lower, "which is missing") || strings.Contains(lower, "is not installed")):
			cause = "dependency"
		case strings.Contains(lower, "requires") && (strings.Contains(lower, "incompatible version") || strings.Contains(lower, "wrong version is present")):
			cause = "mod-version"
		case strings.Contains(lower, "incompatible mods found") || strings.Contains(lower, "incompatible mod set") || strings.Contains(lower, "missing or unsupported mandatory dependencies"):
			if fallback.Cause == "" {
				fallback = Diagnosis{"mod-version", line}
			}
		}
		if cause != "" {
			return Diagnosis{cause, line}
		}
	}
	return fallback
}

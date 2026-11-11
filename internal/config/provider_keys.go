package config

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	launcherCurseForgeKey = "LXMCL_CURSEFORGE_API_KEY"
	standardCurseForgeKey = "CURSEFORGE_API_KEY"
)

// LoadCurseForgeAPIKey reads local configuration without adding it to the process environment.
func LoadCurseForgeAPIKey(dataDir string) string {
	for _, name := range []string{launcherCurseForgeKey, standardCurseForgeKey} {
		if key := strings.TrimSpace(os.Getenv(name)); key != "" {
			return key
		}
	}
	workingDir, _ := os.Getwd()
	exe, _ := os.Executable()
	for _, dir := range []string{dataDir, workingDir, filepath.Dir(exe)} {
		if dir != "" {
			if key := readCurseForgeEnv(filepath.Join(dir, ".env")); key != "" {
				return key
			}
		}
	}
	return ""
}

func readCurseForgeEnv(path string) string {
	const maxEnv = 64 << 10
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() || info.Size() > maxEnv {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxEnv+1))
	if err != nil || len(data) > maxEnv {
		return ""
	}
	var fallback string
	for line := range strings.SplitSeq(strings.TrimPrefix(string(data), "\ufeff"), "\n") {
		name, value, ok := strings.Cut(line, "=")
		name = strings.TrimSpace(name)
		if !ok || (name != launcherCurseForgeKey && name != standardCurseForgeKey) {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && (value[0] == '\'' || value[0] == '"') && value[len(value)-1] == value[0] {
			value = value[1 : len(value)-1]
		}
		if value == "" || strings.ContainsAny(value, "\"'") || strings.ContainsFunc(value, func(r rune) bool { return r <= ' ' || r > '~' }) {
			continue
		}
		if name == launcherCurseForgeKey {
			return value
		}
		fallback = value
	}
	return fallback
}

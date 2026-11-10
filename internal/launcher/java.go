package launcher

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

type Java struct {
	Path  string
	Major int
}

var javaVersion = regexp.MustCompile(`(?m)java\.specification\.version\s*=\s*(?:1\.)?(\d+)`)

func ProbeJava(ctx context.Context, path string) (Java, error) {
	path = strings.TrimSpace(path)
	if len(path) >= 2 && path[0] == '"' && path[len(path)-1] == '"' {
		path = path[1 : len(path)-1]
	}
	path, err := exec.LookPath(path)
	if err != nil {
		return Java{}, i18n.Error("java.lookup_failed", err)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return Java{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// javaw does not reliably write diagnostics to redirected stdout/stderr.
	// Probe its console counterpart while retaining the requested executable.
	probe := javaConsole(path)
	cmd := exec.CommandContext(ctx, probe, "-XshowSettings:properties", "-version")
	cmd.Env = javaEnvironment(false)
	hideConsole(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return Java{}, i18n.Error("java.probe_failed", err)
	}
	m := javaVersion.FindSubmatch(out)
	if len(m) == 0 {
		return Java{}, i18n.Error("java.version_unknown")
	}
	major, err := strconv.Atoi(string(m[1]))
	return Java{Path: path, Major: major}, err
}

func javaConsole(path string) string {
	if runtime.GOOS == "windows" && strings.EqualFold(filepath.Base(path), "javaw.exe") {
		sibling := filepath.Join(filepath.Dir(path), "java.exe")
		if st, err := os.Stat(sibling); err == nil && st.Mode().IsRegular() {
			return sibling
		}
	}
	return path
}

func javaEnvironment(installer bool) []string {
	return slices.DeleteFunc(os.Environ(), func(entry string) bool {
		key, _, _ := strings.Cut(entry, "=")
		key = strings.ToUpper(key)
		switch key {
		case launcherCurseForgeKey, standardCurseForgeKey:
			return true
		case "JAVA_TOOL_OPTIONS", "JDK_JAVA_OPTIONS", "_JAVA_OPTIONS":
			return installer
		}
		return installer && (strings.Contains(key, "KEY") || strings.Contains(key, "TOKEN"))
	})
}

func FindJava(ctx context.Context, preferred string, minimum int) (Java, error) {
	if strings.TrimSpace(preferred) != "" {
		j, err := ProbeJava(ctx, preferred)
		if err == nil && j.Major < minimum {
			err = i18n.Error("java.version_too_old", j.Major, minimum)
		}
		return j, err
	}
	bin := "java"
	if runtime.GOOS == "windows" {
		bin = "java.exe"
	}
	paths := []string{}
	if home := os.Getenv("JAVA_HOME"); home != "" {
		paths = append(paths, filepath.Join(home, "bin", bin))
	}
	paths = append(paths, bin)
	if runtime.GOOS == "windows" {
		if root := os.Getenv("ProgramFiles"); root != "" {
			for _, folder := range []string{"Java", "Eclipse Adoptium", "BellSoft", "Microsoft", "Zulu", "Amazon Corretto"} {
				matches, _ := filepath.Glob(filepath.Join(root, folder, "*", "bin", bin))
				paths = append(paths, matches...)
			}
		}
		if root := os.Getenv("APPDATA"); root != "" {
			matches, _ := filepath.Glob(filepath.Join(root, ".minecraft", "runtime", "*", "*", "*", "bin", bin))
			paths = append(paths, matches...)
			matches, _ = filepath.Glob(filepath.Join(root, ".minecraft", "runtime", "*", "bin", bin))
			paths = append(paths, matches...)
		}
	}
	for _, p := range paths {
		if ctx.Err() != nil {
			return Java{}, ctx.Err()
		}
		if j, err := ProbeJava(ctx, p); err == nil && j.Major >= minimum {
			return j, nil
		}
	}
	if err := ctx.Err(); err != nil {
		return Java{}, err
	}
	return Java{}, i18n.Error("java.compatible_not_found", minimum)
}

// CheckJava resolves the same runtime as Start without downloading the game.
// A new instance only needs the official version metadata.
func (g Game) CheckJava(ctx context.Context, i Instance, globalPath string) (Java, int, error) {
	if err := i.Validate(); err != nil {
		return Java{}, 0, err
	}
	if err := i.Launch.Validate(); err != nil {
		return Java{}, 0, err
	}
	receipt, err := g.readReceipt(i.Version)
	var m metadata
	if err == nil {
		m, err = g.readVersion(i.Version, receipt.VersionSHA)
	}
	if err != nil {
		m, _, err = g.downloadVersion(ctx, i.Version, &downloads{http: officialHTTP})
		if err != nil {
			return Java{}, 0, i18n.Error("java.requirement_read_failed", err)
		}
	}
	minimum := m.JavaVersion.MajorVersion
	j, err := FindJava(ctx, i.Launch.JavaExecutable(globalPath), minimum)
	return j, minimum, err
}

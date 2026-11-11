package javaruntime

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
	"github.com/CAUCraft/LXMCL/internal/process"
)

type Runtime struct {
	Path  string
	Major int
}

var javaVersion = regexp.MustCompile(`(?m)java\.specification\.version\s*=\s*(?:1\.)?(\d+)`)

func Probe(ctx context.Context, path string) (Runtime, error) {
	path = strings.TrimSpace(path)
	if len(path) >= 2 && path[0] == '"' && path[len(path)-1] == '"' {
		path = path[1 : len(path)-1]
	}
	path, err := exec.LookPath(path)
	if err != nil {
		return Runtime{}, i18n.Error("java.lookup_failed", err)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return Runtime{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// javaw does not reliably write diagnostics to redirected stdout/stderr.
	// Probe its console counterpart while retaining the requested executable.
	probe := Console(path)
	cmd := exec.CommandContext(ctx, probe, "-XshowSettings:properties", "-version")
	cmd.Env = Environment(false)
	process.HideConsole(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return Runtime{}, i18n.Error("java.probe_failed", err)
	}
	m := javaVersion.FindSubmatch(out)
	if len(m) == 0 {
		return Runtime{}, i18n.Error("java.version_unknown")
	}
	major, err := strconv.Atoi(string(m[1]))
	return Runtime{Path: path, Major: major}, err
}

func Console(path string) string {
	if runtime.GOOS == "windows" && strings.EqualFold(filepath.Base(path), "javaw.exe") {
		sibling := filepath.Join(filepath.Dir(path), "java.exe")
		if st, err := os.Stat(sibling); err == nil && st.Mode().IsRegular() {
			return sibling
		}
	}
	return path
}

func Environment(installer bool) []string {
	return slices.DeleteFunc(os.Environ(), func(entry string) bool {
		key, _, _ := strings.Cut(entry, "=")
		key = strings.ToUpper(key)
		switch key {
		case "LXMCL_CURSEFORGE_API_KEY", "CURSEFORGE_API_KEY":
			return true
		case "JAVA_TOOL_OPTIONS", "JDK_JAVA_OPTIONS", "_JAVA_OPTIONS":
			return installer
		}
		return installer && (strings.Contains(key, "KEY") || strings.Contains(key, "TOKEN"))
	})
}

func Find(ctx context.Context, preferred string, minimum int) (Runtime, error) {
	if strings.TrimSpace(preferred) != "" {
		j, err := Probe(ctx, preferred)
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
			return Runtime{}, ctx.Err()
		}
		if j, err := Probe(ctx, p); err == nil && j.Major >= minimum {
			return j, nil
		}
	}
	if err := ctx.Err(); err != nil {
		return Runtime{}, err
	}
	return Runtime{}, i18n.Error("java.compatible_not_found", minimum)
}

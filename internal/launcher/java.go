package launcher

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
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
		return Java{}, fmt.Errorf("找不到 Java：%w", err)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return Java{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	probe := path
	// javaw does not reliably write diagnostics to redirected stdout/stderr.
	// Probe its console counterpart while retaining the requested executable.
	if runtime.GOOS == "windows" && strings.EqualFold(filepath.Base(path), "javaw.exe") {
		sibling := filepath.Join(filepath.Dir(path), "java.exe")
		if st, err := os.Stat(sibling); err == nil && st.Mode().IsRegular() {
			probe = sibling
		}
	}
	cmd := exec.CommandContext(ctx, probe, "-XshowSettings:properties", "-version")
	hideConsole(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return Java{}, fmt.Errorf("Java 检测失败：%w", err)
	}
	m := javaVersion.FindSubmatch(out)
	if len(m) == 0 {
		return Java{}, errors.New("无法识别 Java 版本")
	}
	major, err := strconv.Atoi(string(m[1]))
	return Java{Path: path, Major: major}, err
}

func FindJava(ctx context.Context, preferred string, minimum int) (Java, error) {
	if strings.TrimSpace(preferred) != "" {
		j, err := ProbeJava(ctx, preferred)
		if err == nil && j.Major < minimum {
			err = fmt.Errorf("当前是 Java %d，需要 Java %d 或以上", j.Major, minimum)
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
	return Java{}, fmt.Errorf("未找到 Java %d 或以上，请在设置中指定 Java 路径", minimum)
}

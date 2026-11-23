package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNativeLauncherClipboard(t *testing.T) {
	if os.Getenv("RUNNER_ENVIRONMENT") != "github-hosted" {
		t.Skip("requires a disposable Windows desktop and its system clipboard")
	}
	exe := filepath.Join(t.TempDir(), "LXMCL.exe")
	build := exec.CommandContext(t.Context(), filepath.Join(runtime.GOROOT(), "bin", "go.exe"), "build", "-o", exe, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build launcher: %v\n%s", err, output)
	}
	run := exec.CommandContext(t.Context(), "pwsh.exe", "-NoProfile", "-NonInteractive", "-STA", "-File", "testdata/native_clipboard_windows.ps1", exe)
	if output, err := run.CombinedOutput(); err != nil {
		t.Fatalf("native clipboard: %v\n%s", err, output)
	}
}

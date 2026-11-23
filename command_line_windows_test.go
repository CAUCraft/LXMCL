package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/CAUCraft/LXMCL/internal/shortcut"
	"github.com/CAUCraft/LXMCL/internal/tasks"
)

func TestCommandLaunchInExistingWindow(t *testing.T) {
	root := t.TempDir()
	store := launcher.Store{Root: filepath.Join(root, "test profile")}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Home", "Target"} {
		if err := state.Create(name, "1.21.1"); err != nil {
			t.Fatal(err)
		}
	}
	state.Selected = state.Instances[0].ID
	target := state.Instances[1]
	// A synthetic selected profile without saved credentials fails before network or installation.
	state.MicrosoftProfile = "00000000000000000000000000000001"
	if err := store.Save(state); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(root, "Launcher 测试", "LXMCL.exe")
	build := exec.CommandContext(t.Context(), filepath.Join(runtime.GOROOT(), "bin", "go.exe"), "build", "-o", executable, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build launcher: %v, %s", err, out)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	start := func(args ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, executable, args...)
		cmd.Dir = root
		return cmd
	}

	primary := start("--data-dir", store.Root, "--launch", "2")
	var diag bytes.Buffer
	primary.Stdout, primary.Stderr = &diag, &diag
	if err := primary.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cancel()
		_ = primary.Wait()
		if t.Failed() {
			t.Log(diag.String())
		}
	}()
	waitRecords := func(count int) {
		t.Helper()
		for ctx.Err() == nil {
			rows, err := tasks.Load(store.Root)
			if err == nil && len(rows) >= count {
				if len(rows) != count {
					t.Fatal("unexpected launch", rows)
				}
				for _, row := range rows {
					if row.InstanceID != target.ID || row.Revision != target.Revision || row.Status != tasks.Failed ||
						row.Account != state.AccountKey() || row.Action != tasks.Launch || row.PID != 0 {
						t.Fatal("wrong target, account or task", row)
					}
				}
				return
			}
			time.Sleep(25 * time.Millisecond)
		}
		t.Fatalf("timed out waiting for %d launch records", count)
	}
	waitRecords(1)
	for _, args := range [][]string{nil, {"--launch", "99"}, {"--launch", "2"}} {
		if out, err := start(append([]string{"--data-dir", "test profile"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("second process %q: %v, %s", args, err, out)
		}
	}
	waitRecords(2)

	path := filepath.Join(root, "启动 Target.lnk")
	if err := shortcut.Create(ctx, path, executable, store.Root, target.ID); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := shortcut.Create(ctx, path, executable, store.Root, state.Selected); !errors.Is(err, os.ErrExist) {
		t.Fatal("existing link was not protected", err)
	}
	if after, err := os.ReadFile(path); err != nil || !bytes.Equal(before, after) {
		t.Fatal("existing shortcut changed", err)
	}
	// Check the properties as Windows sees them, then invoke the .lnk through the shell.
	const openLink = `$ErrorActionPreference = 'Stop'
$link = (New-Object -ComObject WScript.Shell).CreateShortcut($env:LXMCL_TEST_LINK)
if ($link.TargetPath -ne $env:LXMCL_TEST_TARGET -or $link.WorkingDirectory -ne (Split-Path $env:LXMCL_TEST_TARGET) -or $link.Arguments -cne $env:LXMCL_TEST_ARGS) {
    throw "Unexpected shortcut properties: $($link.TargetPath) | $($link.WorkingDirectory) | $($link.Arguments)"
}
Start-Process -FilePath $env:LXMCL_TEST_LINK -Wait`
	powershell := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	invoke := exec.CommandContext(ctx, powershell, "-NoProfile", "-NonInteractive", "-Command", openLink)
	invoke.Env = append(os.Environ(), "LXMCL_TEST_LINK="+path, "LXMCL_TEST_TARGET="+executable,
		"LXMCL_TEST_ARGS=--data-dir "+syscall.EscapeArg(store.Root)+" --launch "+strconv.Itoa(target.ID))
	if out, err := invoke.CombinedOutput(); err != nil {
		t.Fatalf("open saved shortcut: %v, %s", err, out)
	}
	waitRecords(3)
	if saved, err := store.Load(); err != nil || saved.Selected != state.Selected {
		t.Fatal("command changed home selection", err, saved.Selected)
	}
}

package main

import (
	"bytes"
	"context"
	"encoding/json"
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
	if len(before) < 76 || !bytes.HasPrefix(before, []byte{0x4c, 0, 0, 0, 1, 0x14, 2, 0}) {
		t.Fatalf("missing Shell Link header (%d bytes)", len(before))
	}
	if err := shortcut.Create(ctx, path, executable, store.Root, state.Selected); !errors.Is(err, os.ErrExist) {
		t.Fatal("existing link was not protected", err)
	}
	if after, err := os.ReadFile(path); err != nil || !bytes.Equal(before, after) {
		t.Fatal("existing shortcut changed", err)
	}
	// Check the properties as Windows sees them, then invoke the .lnk through the shell.
	const openLink = `$ErrorActionPreference = 'Stop'
$file = Get-Item -LiteralPath $env:LXMCL_TEST_LINK
$shell = New-Object -ComObject Shell.Application
$item = $shell.NameSpace([string]$file.DirectoryName).ParseName([string]$file.Name)
if (!$item.IsLink) { throw "Explorer did not recognize the saved shortcut" }
$link = $item.GetLink
Start-Process -FilePath $file.FullName -Wait
[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
@{Path=$link.Path; WorkingDirectory=$link.WorkingDirectory; Arguments=$link.Arguments} | ConvertTo-Json -Compress`
	powershell := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	invoke := exec.CommandContext(ctx, powershell, "-NoProfile", "-NonInteractive", "-Command", openLink)
	invoke.Env = append(os.Environ(), "LXMCL_TEST_LINK="+path)
	out, err := invoke.CombinedOutput()
	if err != nil {
		rows, _ := tasks.Load(store.Root)
		t.Fatalf("open saved shortcut (%d journal entries): %v, %s", len(rows), err, out)
	}
	waitRecords(3)
	var link struct{ Path, WorkingDirectory, Arguments string }
	if err := json.Unmarshal(out, &link); err != nil {
		t.Fatal(err, string(out))
	}
	// Explorer may expand an 8.3 path (such as RUNNER~1) to its long name.
	for _, pair := range [][2]string{{link.Path, executable}, {link.WorkingDirectory, filepath.Dir(executable)}} {
		actual, want := pair[0], pair[1]
		got, err := os.Stat(actual)
		expected, wantErr := os.Stat(want)
		if err != nil || wantErr != nil || !os.SameFile(got, expected) {
			t.Fatalf("shortcut path %q does not identify %q", actual, want)
		}
	}
	if link.Arguments != "--data-dir "+syscall.EscapeArg(store.Root)+" --launch "+strconv.Itoa(target.ID) {
		t.Fatal("unexpected shortcut arguments", link.Arguments)
	}
	if saved, err := store.Load(); err != nil || saved.Selected != state.Selected {
		t.Fatal("command changed home selection", err, saved.Selected)
	}
}

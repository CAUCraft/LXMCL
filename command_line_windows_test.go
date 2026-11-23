package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/CAUCraft/LXMCL/internal/tasks"
)

// Run the normal GUI entry point in child processes on the Windows desktop runner.
func TestMain(m *testing.M) {
	if os.Getenv("LXMCL_TEST_CLI_PROCESS") == "1" {
		main()
		return
	}
	os.Exit(m.Run())
}

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
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	start := func(args ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0], args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "LXMCL_TEST_CLI_PROCESS=1")
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
	if saved, err := store.Load(); err != nil || saved.Selected != state.Selected {
		t.Fatal("command changed home selection", err, saved.Selected)
	}
}

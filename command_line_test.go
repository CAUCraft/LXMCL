package main

import (
	"errors"
	"flag"
	"strings"
	"testing"
)

func TestCommandLine(t *testing.T) {
	cmd, err := parseCommandLine(nil)
	if err != nil || cmd != (commandLine{}) {
		t.Fatalf("ordinary startup: %+v, %v", cmd, err)
	}
	cmd, err = parseCommandLine([]string{"-data-dir", "local test", "--launch=27"})
	if err != nil || cmd != (commandLine{dataDir: "local test", launchID: 27}) {
		t.Fatalf("isolated launch: %+v, %v", cmd, err)
	}
	cmd, err = parseCommandLine([]string{"--launch", "002"})
	if err != nil || cmd.launchID != 2 {
		t.Fatalf("decimal ID: %+v, %v", cmd, err)
	}
	if _, err := parseCommandLine([]string{"--help"}); !errors.Is(err, flag.ErrHelp) {
		t.Fatal("help did not exit before startup")
	}
	for _, args := range [][]string{
		{"--launch"}, {"--launch", "0"}, {"--launch", "-1"}, {"--launch", "0xff"},
		{"--launch", "18446744073709551616"}, {"--launch", "1", "--launch", "2"},
		{"--launch", "2", "extra"}, {"--launch", "1", "--username", "example"},
		{"--launch", "2", "--", "extra"}, {"--command", "extra"},
	} {
		if got, err := parseCommandLine(args); err == nil || got != (commandLine{}) {
			t.Fatalf("accepted %q: %+v", args, got)
		}
	}
}

func TestCommandLaunchDoesNotRetarget(t *testing.T) {
	a, _ := testDesktop(t)
	a.ready = false
	a.secondInstance([]string{"--launch", "2"}, "")
	a.secondInstance(nil, "")
	a.secondInstance([]string{"--launch", "1"}, "")
	if a.pendingLaunchID != 2 {
		t.Fatal("a later invocation replaced the startup target")
	}
	a.ready, a.pendingLaunchID = true, 0
	a.launchByID(20)
	if !a.noticeError || !strings.Contains(a.notice, "20") {
		t.Fatal("missing ID was not explained", a.notice)
	}
	a.busy = true
	a.launchByID(2)
	if !a.noticeError || !strings.Contains(a.notice, "重试") || a.pendingLaunchID != 0 || a.state.Selected != 1 || len(a.history) != 0 {
		t.Fatal("blocked command was queued or changed the target", a.notice)
	}
}

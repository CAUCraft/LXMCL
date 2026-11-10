package storage

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestPublishNewCrashLeavesRetryPossible(t *testing.T) {
	const childDirectory = "LXMCL_TEST_PUBLISH_CRASH_DIRECTORY"
	if dir := os.Getenv(childDirectory); dir != "" {
		root, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		err = PublishNew(t.Context(), root, "file.zip", 0644, func(f *os.File) error {
			if _, err := f.WriteString("unfinished"); err != nil {
				return err
			}
			// Exit without running deferred cleanup, as in a process crash.
			os.Exit(91)
			return nil
		})
		t.Fatalf("child failed to exit during preparation: %v", err)
	}
	dir := t.TempDir()
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestPublishNewCrashLeavesRetryPossible$")
	cmd.Env = append(os.Environ(), childDirectory+"="+dir)
	output, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 91 {
		t.Fatalf("child exit: %v\n%s", err, output)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || !strings.HasSuffix(entries[0].Name(), ".part") {
		t.Fatalf("crash exposed a final name: %v, %v", entries, err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	err = PublishNew(t.Context(), root, "file.zip", 0644, func(f *os.File) error {
		_, err := f.WriteString("verified retry")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := root.ReadFile("file.zip")
	if err != nil || string(data) != "verified retry" {
		t.Fatalf("retry failed: %q, %v", data, err)
	}
	// A retry owns only its own staging file; it must not remove earlier files.
	data, err = root.ReadFile(entries[0].Name())
	if err != nil || string(data) != "unfinished" {
		t.Fatalf("unowned staging file changed: %q, %v", data, err)
	}
}

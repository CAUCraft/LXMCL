package minecraft_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/CAUCraft/LXMCL/internal/account"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/minecraft"
	"github.com/CAUCraft/LXMCL/internal/testutil/mcfixture"
)

// The test binary supplies a deterministic Java probe and a real child process.
func TestMain(m *testing.M) {
	if os.Getenv("LXMCL_RUNTIME_PROCESS") == "1" && len(os.Args) > 1 {
		switch os.Args[1] {
		case "-XshowSettings:properties":
			fmt.Println("    java.specification.version = 21")
			os.Exit(0)
		case "-Xmx2048M":
			main, agent := slices.Index(os.Args, "test.Main"), slices.Index(os.Args, "-Dprobe=private-value")
			if agent < 0 || main < agent {
				os.Exit(2)
			}
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				if _, err := os.Stat("release-process"); err == nil {
					fmt.Println("runtime process completed")
					os.Exit(0)
				}
				time.Sleep(10 * time.Millisecond)
			}
			os.Exit(3)
		}
	}
	os.Exit(m.Run())
}

func TestPublicRuntimeKeepsAuthenticationPreparationLazyAndProcessIndependent(t *testing.T) {
	t.Setenv("LXMCL_RUNTIME_PROCESS", "1")
	fixture := mcfixture.New(t)
	runtime := minecraft.Runtime{Cache: t.TempDir(), Client: minecraft.Client{Transport: fixture}}
	i := instance.Instance{Version: fixture.Version.ID, GameDir: t.TempDir()}
	if err := runtime.Install(t.Context(), i, nil); err != nil {
		t.Fatal(err)
	}
	if !runtime.Installed(i) {
		t.Fatal("public install did not publish a receipt")
	}
	identity, err := account.OfflineIdentity("Player")
	if err != nil {
		t.Fatal(err)
	}
	java, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	called := 0
	prepareErr := errors.New("authentication agent unavailable")
	options := minecraft.LaunchOptions{Identity: identity, Settings: instance.Settings{Memory: 2048, JavaPath: filepath.Join(t.TempDir(), "missing-java")},
		PrepareJVM: func(context.Context) ([]string, error) { called++; return nil, prepareErr },
	}
	if _, err := runtime.Start(t.Context(), i, options, nil); !errors.Is(err, minecraft.ErrJavaRequired) || called != 0 {
		t.Fatalf("preparation before Java check: %v, calls %d", err, called)
	}
	options.Settings.JavaPath = java
	if run, err := runtime.Start(t.Context(), i, options, nil); !errors.Is(err, prepareErr) || called != 1 || run.PID != 0 {
		t.Fatalf("preparation failure lost: %+v %v calls %d", run, err, called)
	}
	ctx, cancel := context.WithCancel(t.Context())
	options.PrepareJVM = func(context.Context) ([]string, error) { cancel(); return nil, nil }
	if run, err := runtime.Start(ctx, i, options, nil); !errors.Is(err, context.Canceled) || run.PID != 0 {
		t.Fatalf("cancellation started a process: %+v %v", run, err)
	}
	options.PrepareJVM = func(context.Context) ([]string, error) { return []string{"-Dprobe=private-value"}, nil }
	ctx, cancel = context.WithCancel(t.Context())
	defer cancel()
	run, err := runtime.Start(ctx, i, options, nil)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := os.WriteFile(filepath.Join(i.GameDir, "release-process"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-run.Done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("child process did not complete")
	}
	log, err := os.ReadFile(run.Log)
	if err != nil || !strings.Contains(string(log), "runtime process completed") || strings.Contains(string(log), "private-value") {
		t.Fatalf("unexpected process log: %s, %v", log, err)
	}
	options.Identity = account.LaunchIdentity{}
	if _, err := runtime.Start(t.Context(), i, options, nil); err == nil {
		t.Fatal("empty identity implicitly launched offline")
	}
}

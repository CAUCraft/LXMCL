package minecraft

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"time"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/javaruntime"
	launcherlog "github.com/CAUCraft/LXMCL/internal/log"
	"github.com/CAUCraft/LXMCL/internal/process"
)

func renameForgeCache(ctx context.Context, from, to string) error {
	var err error
	for attempt := range 6 {
		if err := ctx.Err(); err != nil {
			return err
		}
		err = os.Rename(from, to)
		if err == nil || runtime.GOOS != "windows" || attempt == 5 ||
			(!errors.Is(err, os.ErrPermission) && !errors.Is(err, syscall.Errno(32))) {
			return err
		}
		// Windows scanners may briefly hold newly installed JARs open.
		if err := waitContext(ctx, time.Duration(1<<attempt)*50*time.Millisecond); err != nil {
			return err
		}
	}
	return err
}

func (g Runtime) runForgeInstaller(ctx context.Context, java, installer, game string, i instance.Instance) error {
	log, err := launcherlog.CreateInstaller(g.Cache, i.Loader)
	if err != nil {
		return err
	}
	defer log.Close()
	ctx, stop := context.WithTimeout(ctx, 15*time.Minute)
	defer stop()
	cmd := exec.CommandContext(ctx, java, "-Djava.awt.headless=true", "-Duser.home="+game,
		"-jar", installer, "--installClient", game)
	process.HideConsole(cmd)
	cmd.Dir, cmd.Stdout, cmd.Stderr = game, log, log
	// Do not pass key/token environment variables or inherited Java agents.
	cmd.Env = javaruntime.Environment(true)
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return context.Canceled
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return i18n.Error("forge.installer_timed_out", i.LoaderName(), log.Name(), ctx.Err())
		}
		return i18n.Error("forge.installer_process_failed", i.LoaderName(), err, log.Name())
	}
	return nil
}

package minecraft

import (
	"context"
	"errors"
	"os/exec"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/javaruntime"
	launcherlog "github.com/CAUCraft/LXMCL/internal/log"
	"github.com/CAUCraft/LXMCL/internal/process"
)

func startProcess(ctx context.Context, i instance.Instance, j javaruntime.Runtime, memory int, args []string) (Running, error) {
	log, err := launcherlog.CreateGame(i.GameDir, launcherlog.GameHeader{
		Version: i.Version, JavaPath: j.Path, JavaMajor: j.Major, MemoryMiB: memory,
	})
	if err != nil {
		return Running{}, err
	}
	// The game process owns its own lifetime after Start, independent of the launcher window.
	cmd := exec.Command(j.Path, args...)
	process.HideConsole(cmd)
	cmd.Dir, cmd.Stdout, cmd.Stderr = i.GameDir, log, log
	cmd.Env = javaruntime.Environment(false)
	if err := ctx.Err(); err != nil {
		log.Close()
		return Running{}, err
	}
	if err := cmd.Start(); err != nil {
		log.Close()
		return Running{}, i18n.Error("launch.process_start_failed", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- errors.Join(cmd.Wait(), log.Close())
		close(done)
	}()
	return Running{PID: cmd.Process.Pid, Log: log.Name(), Done: done}, nil
}

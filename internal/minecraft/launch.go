package minecraft

import (
	"context"

	"github.com/CAUCraft/LXMCL/internal/account"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

// LaunchOptions carries a validated identity. PrepareJVM lazily obtains optional
// authentication agents after file and Java checks; it must honor cancellation.
// Neither the identity argument map nor the returned JVM arguments may be logged.
type LaunchOptions struct {
	Settings   instance.Settings
	Server     string
	Identity   account.LaunchIdentity
	PrepareJVM func(context.Context) ([]string, error)
}

type Running struct {
	PID  int
	Log  string
	Done <-chan error
}

// Start verifies the installation before preparing authentication JVM arguments.
// Success means a Java process started; callers observe Done for its exit status.
func (g Runtime) Start(ctx context.Context, i instance.Instance, options LaunchOptions, report func(Progress)) (Running, error) {
	settings := options.Settings
	if err := ctx.Err(); err != nil {
		return Running{}, err
	}
	if options.Identity == (account.LaunchIdentity{}) {
		return Running{}, i18n.Error("launch.identity_required")
	}
	if err := i.Validate(); err != nil {
		return Running{}, err
	}
	if err := i.Launch.Validate(); err != nil {
		return Running{}, err
	}
	memory := i.Launch.MaxMemory(settings.Memory)
	if err := instance.ValidateMemory(memory); err != nil {
		return Running{}, err
	}
	profile, err := g.prepareProfile(ctx, i)
	if err != nil {
		if ctx.Err() != nil {
			return Running{}, ctx.Err()
		}
		return Running{}, i18n.Error("common.wrapped_errors", ErrRepairRequired, err)
	}
	if err := validateClasspath(profile.classpath); err != nil {
		return Running{}, err
	}
	j, err := g.resolveJava(ctx, i.Launch.JavaExecutable(settings.JavaPath), profile.metadata, report)
	if err != nil {
		if ctx.Err() != nil {
			return Running{}, ctx.Err()
		}
		return Running{}, i18n.Error("common.wrapped_errors", ErrJavaRequired, err)
	}
	if err := ctx.Err(); err != nil {
		return Running{}, err
	}
	args, err := g.launchArguments(ctx, i, options, profile, memory)
	if err != nil {
		return Running{}, err
	}
	return startProcess(ctx, i, j, memory, args)
}

var (
	// ErrRepairRequired indicates that cached files cannot be used to start the client.
	ErrRepairRequired = i18n.Error("launch.repair_required")
	// ErrJavaRequired indicates that the selected game needs a usable Java runtime.
	ErrJavaRequired = i18n.Error("launch.java_required")
)

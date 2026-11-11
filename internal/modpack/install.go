package modpack

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/distribution"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/tasks"
)

// InstallOptions supplies application services without coupling archives to launcher state.
type InstallOptions struct {
	Target   instance.Instance
	Cache    distribution.Cache
	Resolver Resolver
	// InstallGame installs and verifies the game in the supplied staging instance.
	InstallGame func(context.Context, instance.Instance, func(tasks.Progress)) error
	// Commit saves the captured instance configuration after files are published.
	// It must be serialized with other state writes and leave the old state on failure.
	Commit func() error
}

// Install validates the preview and files, installs in a temporary directory, then
// publishes the instance. A failed Commit rolls the directory back for cleanup.
// It runs on the calling goroutine; progress is reported from worker goroutines.
func Install(ctx context.Context, pack Prepared, options InstallOptions, report func(tasks.Progress)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if options.InstallGame == nil || options.Commit == nil {
		return i18n.Error("modpack.install_callbacks_required")
	}
	i := options.Target
	if i.Version != pack.info.Minecraft || !strings.EqualFold(i.Loader, pack.info.Loader) ||
		i.LoaderVersion != pack.info.LoaderVersion || !filepath.IsAbs(i.GameDir) {
		return i18n.Error("modpack.instance_changed")
	}
	if err := i.Validate(); err != nil {
		return err
	}
	if report == nil {
		report = func(tasks.Progress) {}
	}
	report(tasks.Progress{Stage: i18n.Text("progress.check_modpack_manifest")})
	f, size, err := openArchive(pack.filename)
	if err != nil {
		return err
	}
	defer f.Close()
	sum, err := fingerprint(ctx, f, size)
	if err != nil {
		return err
	}
	if sum != pack.sum {
		return i18n.Error("modpack.archive_changed")
	}
	plan, err := readPlan(f, size, pack.info.Source)
	if err != nil {
		return err
	}
	if len(plan.curse) != 0 {
		if options.Resolver == nil {
			return i18n.Error("modpack.resolver_required")
		}
		report(tasks.Progress{Stage: i18n.Text("progress.resolve_curseforge_dependencies")})
		plan.downloads, err = options.Resolver.Resolve(ctx, plan.curse)
		if err != nil {
			return err
		}
		if len(plan.downloads) != len(plan.curse) {
			return i18n.Error("modpack.dependencies_incomplete")
		}
		if err := plan.validate(); err != nil {
			return err
		}
	}
	parent, dest := filepath.Dir(i.GameDir), filepath.Base(i.GameDir)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return err
	}
	base, err := os.OpenRoot(parent)
	if err != nil {
		return err
	}
	defer base.Close()
	checkDest := func() error {
		if _, err := base.Lstat(dest); !errors.Is(err, os.ErrNotExist) {
			if err == nil {
				err = i18n.Error("modpack.instance_directory_exists")
			}
			return err
		}
		return nil
	}
	if err := checkDest(); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(parent, ".lxmcl-modpack-")
	if err != nil {
		return err
	}
	stage := filepath.Base(tmp)
	defer base.RemoveAll(stage)
	root, err := base.OpenRoot(stage)
	if err != nil {
		return err
	}
	err = fill(ctx, root, plan, options.Cache, report)
	if err := errors.Join(err, root.Close()); err != nil {
		return err
	}
	work := i
	work.GameDir = tmp
	if err := options.InstallGame(ctx, work, report); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	report(tasks.Progress{Stage: i18n.Text("progress.save_instance")})
	if err := ctx.Err(); err != nil {
		return err
	}
	// Recheck after callbacks, immediately before publication. The application
	// serializes installations and state commits for this game root.
	if err := checkDest(); err != nil {
		return err
	}
	if err := base.Rename(stage, dest); err != nil {
		return err
	}
	if err := options.Commit(); err != nil {
		return errors.Join(err, base.Rename(dest, stage))
	}
	return nil
}

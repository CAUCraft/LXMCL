package launcher

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/tasks"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

// InstallModpack installs and validates all files before publishing the instance.
// The caller serializes state writes and keeps the submitted state snapshot.
func (g Game) InstallModpack(ctx context.Context, store Store, next State, i Instance, pack PreparedModpack, key string, report func(Progress)) error {
	install := func(ctx context.Context, work Instance, report func(Progress)) error {
		return g.InstallWithJava(ctx, work, next.Settings.JavaPath, report)
	}
	return tasks.Run(ctx, func(ctx context.Context, progress func(Progress)) error {
		return g.installModpack(ctx, store, next, i, pack, newCurseForge(key), modpackHTTP, install, progress)
	}, report)
}

func (g Game) installModpack(ctx context.Context, store Store, next State, i Instance, pack PreparedModpack, cf curseForge,
	client *http.Client, install func(context.Context, Instance, func(Progress)) error, report func(Progress)) error {
	saved, exists := next.Find(i.ID)
	if !exists || saved != i || i.Version != pack.info.Minecraft || !strings.EqualFold(i.Loader, pack.info.Loader) ||
		i.LoaderVersion != pack.info.LoaderVersion || !filepath.IsAbs(i.GameDir) {
		return i18n.Error("modpack.instance_changed")
	}
	if err := i.Validate(); err != nil {
		return err
	}
	if report == nil {
		report = func(Progress) {}
	}
	report(Progress{Stage: i18n.Text("progress.check_modpack_manifest")})
	f, size, err := openModpack(pack.filename)
	if err != nil {
		return err
	}
	defer f.Close()
	sum, err := modpackFingerprint(ctx, f, size)
	if err != nil {
		return err
	}
	if sum != pack.sum {
		return i18n.Error("modpack.archive_changed")
	}
	plan, err := readModpack(f, size, pack.info.Source)
	if err != nil {
		return err
	}
	if len(plan.curse) != 0 {
		report(Progress{Stage: i18n.Text("progress.resolve_curseforge_dependencies")})
		plan.downloads, err = cf.resolve(ctx, plan.curse)
		if err != nil {
			return err
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
	err = g.fillModpack(ctx, root, plan, client, report)
	if err := errors.Join(err, root.Close()); err != nil {
		return err
	}
	work := i
	work.GameDir = tmp
	if err := install(ctx, work, report); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := checkDest(); err != nil {
		return err
	}
	report(Progress{Stage: i18n.Text("progress.save_instance")})
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := base.Rename(stage, dest); err != nil {
		return err
	}
	if err := store.Save(next); err != nil {
		return errors.Join(err, base.Rename(dest, stage))
	}
	return nil
}

func (g Game) fillModpack(ctx context.Context, root *os.Root, p modpackPlan, client *http.Client, report func(Progress)) error {
	write := func(name string, overwrite bool, copy func(io.Writer) error) error {
		if err := root.MkdirAll(filepath.FromSlash(path.Dir(name)), 0755); err != nil {
			return err
		}
		flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
		if overwrite {
			flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
		}
		out, err := root.OpenFile(filepath.FromSlash(name), flags, 0644)
		if err != nil {
			return err
		}
		return errors.Join(copy(out), out.Close())
	}
	stage := i18n.Text("progress.download_modpack_dependencies")
	report(Progress{Stage: stage, Total: len(p.downloads)})
	if err := transfer.Batch(ctx, p.downloads, func(ctx context.Context, d modpackDownload) error {
		file, err := g.modpackAsset(ctx, d.asset, client)
		if err == nil {
			err = write(d.path, false, func(out io.Writer) error {
				f, err := os.Open(file)
				if err != nil {
					return err
				}
				defer f.Close()
				return d.asset.copyVerified(ctx, out, f)
			})
		}
		if err != nil {
			return i18n.Error("common.context_error", d.path, err)
		}
		return nil
	}, func(done int) { report(Progress{Stage: stage, Done: done, Total: len(p.downloads)}) }); err != nil {
		return err
	}
	stage = i18n.Text("progress.extract_modpack_overrides")
	report(Progress{Stage: stage, Total: len(p.overrides)})
	for n, o := range p.overrides {
		err := write(o.path, true, func(out io.Writer) error {
			r, err := o.file.Open()
			if err != nil {
				return err
			}
			defer r.Close()
			size := int64(o.file.UncompressedSize64)
			count, err := io.Copy(out, io.LimitReader(transfer.Reader(ctx, r), size+1))
			if err == nil && count != size {
				err = i18n.Error("modpack.override_size_mismatch")
			}
			return err
		})
		if err != nil {
			return i18n.Error("common.context_error", o.path, err)
		}
		report(Progress{Stage: stage, Done: n + 1, Total: len(p.overrides)})
	}
	return ctx.Err()
}

package modpack

import (
	"context"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/distribution"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/tasks"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

func fill(ctx context.Context, root *os.Root, p plan, cache distribution.Cache, report func(tasks.Progress)) error {
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
	report(tasks.Progress{Stage: stage, Total: len(p.downloads)})
	if err := transfer.Batch(ctx, p.downloads, func(ctx context.Context, d Download) error {
		file, err := cache.Ensure(ctx, d.Asset)
		if err == nil {
			err = write(d.Path, false, func(out io.Writer) error {
				f, err := os.Open(file)
				if err != nil {
					return err
				}
				defer f.Close()
				return d.Asset.CopyVerified(ctx, out, f)
			})
		}
		if err != nil {
			return i18n.Error("common.context_error", d.Path, err)
		}
		return nil
	}, func(done int) { report(tasks.Progress{Stage: stage, Done: done, Total: len(p.downloads)}) }); err != nil {
		return err
	}
	stage = i18n.Text("progress.extract_modpack_overrides")
	report(tasks.Progress{Stage: stage, Total: len(p.overrides)})
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
		report(tasks.Progress{Stage: stage, Done: n + 1, Total: len(p.overrides)})
	}
	return ctx.Err()
}

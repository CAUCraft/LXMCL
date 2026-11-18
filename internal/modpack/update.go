package modpack

import (
	"context"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/CAUCraft/LXMCL/internal/tasks"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

type Change struct {
	Path, Action string
	before       string
}
type UpdatePreview struct {
	Previous string
	Tracked  bool
	Changes  []Change
	pack     Prepared
	target   instance.Instance
	receipt  string
}

// Stage through the same verified installation path used for a fresh instance.
func withUpdateFiles(ctx context.Context, pack Prepared, o InstallOptions, report func(tasks.Progress), use func(*os.Root, *os.Root) error) error {
	target, err := os.OpenRoot(o.Target.GameDir)
	if err != nil {
		return err
	}
	defer target.Close()
	dir, err := os.MkdirTemp(filepath.Dir(o.Target.GameDir), ".lxmcl-preview-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	work := o.Target
	work.GameDir = filepath.Join(dir, "pack")
	work.Version, work.Loader, work.LoaderVersion = pack.info.Minecraft, pack.info.Loader, pack.info.LoaderVersion
	o.Target = work
	o.Commit = func() error { return nil }
	if o.InstallGame == nil {
		o.InstallGame = func(context.Context, instance.Instance, func(tasks.Progress)) error { return nil }
	}
	if err := Install(ctx, pack, o, report); err != nil {
		return err
	}
	stage, err := os.OpenRoot(work.GameDir)
	if err != nil {
		return err
	}
	defer stage.Close()
	return use(target, stage)
}

func compareUpdate(ctx context.Context, target, stage *os.Root, pack Prepared, i instance.Instance) (UpdatePreview, error) {
	preview := UpdatePreview{pack: pack, target: i}
	sum, err := fileDigest(ctx, target, receiptName)
	if err != nil {
		return preview, err
	}
	preview.receipt = sum
	old, err := readReceipt(target)
	if err != nil {
		return preview, err
	}
	next, err := readReceipt(stage)
	if err != nil {
		return preview, err
	}
	preview.Tracked, preview.Previous = old.Format == 1, old.Version
	names := map[string]string{}
	var ordered []string
	for _, files := range []map[string]string{old.Files, next.Files} {
		for name := range files {
			k := strings.ToLower(name)
			if prior := names[k]; prior != "" {
				if prior != name {
					return preview, i18n.Error("modpack.plan_path_conflict", name)
				}
				continue
			}
			names[k] = name
			ordered = append(ordered, name)
		}
	}
	slices.Sort(ordered)
	for _, name := range ordered {
		if err := ctx.Err(); err != nil {
			return preview, err
		}
		current, err := fileDigest(ctx, target, name)
		if err != nil {
			return preview, err
		}
		before, after := old.Files[name], next.Files[name]
		change := Change{Path: name, before: current}
		switch {
		case after == current:
			continue
		case current == "" && before == "":
			if strings.HasSuffix(name, ".jar") {
				disabled, err := fileDigest(ctx, target, name+".disabled")
				if err != nil {
					return preview, err
				}
				if disabled != "" {
					change.Action = "keep"
					break
				}
			}
			change.Action = "add"
		case current == "" || current != before:
			change.Action = "keep"
		case after == "":
			change.Action = "remove"
		default:
			change.Action = "replace"
		}
		preview.Changes = append(preview.Changes, change)
	}
	return preview, nil
}
func PreviewUpdate(ctx context.Context, pack Prepared, o InstallOptions) (UpdatePreview, error) {
	var result UpdatePreview
	o.InstallGame = nil
	err := withUpdateFiles(ctx, pack, o, nil, func(target, stage *os.Root) error {
		var err error
		result, err = compareUpdate(ctx, target, stage, pack, o.Target)
		return err
	})
	return result, err
}

func ApplyUpdate(ctx context.Context, expected UpdatePreview, o InstallOptions, report func(tasks.Progress)) error {
	if o.Target != expected.target || o.Commit == nil {
		return i18n.Error("modpack.instance_changed")
	}
	return withUpdateFiles(ctx, expected.pack, o, report, func(target, stage *os.Root) error {
		latest, err := compareUpdate(ctx, target, stage, expected.pack, o.Target)
		if err != nil {
			return err
		}
		if latest.receipt != expected.receipt || !slices.Equal(expected.Changes, latest.Changes) {
			return i18n.Error("modpack.preview_changed")
		}
		changes := slices.Clone(latest.Changes)
		changes = append(changes, Change{Path: receiptName, Action: "replace", before: latest.receipt})
		return publishUpdate(ctx, target, stage, changes, o.Commit)
	})
}

func publishUpdate(ctx context.Context, target, stage *os.Root, changes []Change, commit func() error) (result error) {
	temp, err := os.MkdirTemp(target.Name(), ".lxmcl-rollback-")
	if err != nil {
		return err
	}
	backup := filepath.Base(temp)
	type undo struct {
		name             string
		saved, published bool
	}
	var journal []undo
	defer func() {
		var rollback error
		if result != nil {
			for _, step := range slices.Backward(journal) {
				if step.published {
					rollback = errors.Join(rollback, target.Remove(step.name))
				}
				if step.saved {
					rollback = errors.Join(rollback, target.Rename(path.Join(backup, step.name), step.name))
				}
			}
		}
		if rollback != nil {
			result = &RollbackError{Directory: temp, Err: errors.Join(result, rollback, i18n.Error("modpack.rollback_directory", temp))}
			return
		}
		target.RemoveAll(backup)
	}()
	for _, step := range changes {
		if step.Action == "keep" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		current, err := fileDigest(ctx, target, step.Path)
		if err != nil {
			return err
		}
		if current != step.before {
			return i18n.Error("modpack.preview_changed")
		}
		journal = append(journal, undo{name: step.Path})
		entry := &journal[len(journal)-1]
		if current != "" {
			dest := path.Join(backup, step.Path)
			if err := target.MkdirAll(path.Dir(dest), 0700); err != nil {
				return err
			}
			if err := target.Rename(step.Path, dest); err != nil {
				return err
			}
			entry.saved = true
		}
		if step.Action == "remove" {
			continue
		}
		dir := path.Dir(step.Path)
		if err := target.MkdirAll(dir, 0755); err != nil {
			return err
		}
		parent, err := target.OpenRoot(dir)
		if err != nil {
			return err
		}
		err = storage.PublishNew(ctx, parent, path.Base(step.Path), 0644, func(f *os.File) error {
			src, err := stage.Open(step.Path)
			if err != nil {
				return err
			}
			defer src.Close()
			_, err = io.Copy(f, transfer.Reader(ctx, src))
			return err
		})
		parent.Close()
		if err != nil {
			return err
		}
		entry.published = true
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return commit()
}

func (u UpdatePreview) Matches(p Prepared, i instance.Instance) bool {
	return u.target == i && u.pack == p
}

type RollbackError struct {
	Directory string
	Err       error
}

func (e *RollbackError) Error() string { return e.Err.Error() }
func (e *RollbackError) Unwrap() error { return e.Err }

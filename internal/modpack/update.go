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

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/CAUCraft/LXMCL/internal/tasks"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

type Change struct {
	Path, Action     string
	before           string
	source, opposite string
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
	// Index toggled resources by their enabled name. Receipts keep the path
	// supplied by the pack while changes record the actual path on disk.
	type paths struct{ key, alternate, previous, next string }
	files := make(map[string]paths)
	var ordered []string
	for pass, receiptFiles := range []map[string]string{old.Files, next.Files} {
		for name := range receiptFiles {
			key, other := resourcePaths(name)
			if old.Files[name] != "" && old.Files[other] != "" || next.Files[name] != "" && next.Files[other] != "" {
				key, other = name, ""
			}
			folded := strings.ToLower(key)
			p, exists := files[folded]
			if exists && p.key != key || pass == 0 && p.previous != "" || pass == 1 && p.next != "" {
				return preview, i18n.Error("modpack.plan_path_conflict", name)
			}
			if !exists {
				p.key = key
				ordered = append(ordered, folded)
			}
			if pass == 0 {
				p.previous, p.alternate = name, other
			} else {
				p.next, p.alternate = name, other
			}
			files[folded] = p
		}
	}
	slices.Sort(ordered)
	for _, key := range ordered {
		if err := ctx.Err(); err != nil {
			return preview, err
		}
		p := files[key]
		dest := p.next
		if dest == "" {
			dest = p.previous
		}
		current, err := fileDigest(ctx, target, dest)
		if err != nil {
			return preview, err
		}
		change := Change{Path: dest, source: p.next, before: current}
		if alternate := p.alternate; alternate != "" {
			other, err := fileDigest(ctx, target, alternate)
			if err != nil {
				return preview, err
			}
			switch {
			case current != "" && other != "":
				change.Action = "keep"
				preview.Changes = append(preview.Changes, change)
				continue
			case other != "":
				change.Path, change.opposite, change.before = alternate, dest, other
			default:
				change.opposite = alternate
			}
		}
		before, after := old.Files[p.previous], next.Files[p.next]
		switch {
		case change.before == after:
			continue
		case change.before == "" && before == "":
			change.Action = "add"
		case change.before == "" || change.before != before:
			change.Action = "keep"
		case after == "":
			change.Action = "remove"
		default:
			change.Action = "replace"
		}
		preview.Changes = append(preview.Changes, change)
	}
	nextInstance := i
	nextInstance.Version, nextInstance.Loader = pack.info.Minecraft, pack.info.Loader
	err = preserveRenamedMods(ctx, preview.Changes, target, stage, i, nextInstance)
	return preview, err
}

func resourcePaths(name string) (key, alternate string) {
	base, disabled := strings.CutSuffix(name, ".disabled")
	folder, ext := path.Dir(base), strings.ToLower(path.Ext(base))
	switch {
	case folder == "mods" && ext == ".jar", (folder == "resourcepacks" || folder == "shaderpacks") && ext == ".zip":
		if disabled {
			return base, base
		}
		return base, base + ".disabled"
	default:
		return name, ""
	}
}

// A renamed JAR inherits its toggle only when one removed and one added JAR
// declare the same set of mod IDs. File names alone do not establish identity.
func preserveRenamedMods(ctx context.Context, changes []Change, target, stage *os.Root, old, next instance.Instance) error {
	var removed, added []int
	hasDisabled := false
	for n, step := range changes {
		if step.opposite == "" || path.Dir(step.Path) != "mods" {
			continue
		}
		switch step.Action {
		case "remove":
			removed = append(removed, n)
		case "add":
			added = append(added, n)
		default:
			continue
		}
		hasDisabled = hasDisabled || strings.HasSuffix(step.Path, ".disabled")
	}
	if !hasDisabled || len(added) == 0 {
		return nil
	}
	index := func(root *os.Root, i instance.Instance, candidates []int) (map[string]int, error) {
		_, markers, err := content.Mods.Format(i)
		if err != nil {
			return nil, nil
		}
		unique := make(map[string]int)
		for _, n := range candidates {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			_, ids := content.ReadModMetadata(root, changes[n].Path, markers)
			if ids == "" {
				continue
			}
			set := strings.Split(ids, ", ")
			slices.Sort(set)
			key := strings.Join(set, ", ")
			if _, seen := unique[key]; seen {
				unique[key] = -1
			} else {
				unique[key] = n
			}
		}
		return unique, nil
	}
	previous, err := index(target, old, removed)
	if err != nil {
		return err
	}
	incoming, err := index(stage, next, added)
	if err != nil {
		return err
	}
	for ids, n := range incoming {
		last, ok := previous[ids]
		if n < 0 || !ok || last < 0 {
			continue
		}
		if step := &changes[n]; strings.HasSuffix(step.Path, ".disabled") != strings.HasSuffix(changes[last].Path, ".disabled") {
			step.Path, step.opposite = step.opposite, step.Path
		}
	}
	return nil
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
		changes = append(changes, Change{Path: receiptName, source: receiptName, Action: "replace", before: latest.receipt})
		return publishUpdate(ctx, target, stage, changes, o.Commit)
	})
}

// Record each directory as soon as it is created, including parents of a later failed write.
func makeUpdateDirs(target *os.Root, dir string, created *[]string) error {
	if dir == "." {
		return nil
	}
	if _, err := target.Lstat(dir); !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := makeUpdateDirs(target, path.Dir(dir), created); err != nil {
		return err
	}
	if err := target.Mkdir(dir, 0755); err != nil {
		return err
	}
	*created = append(*created, dir)
	return nil
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
	var createdDirs []string
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
			for _, dir := range slices.Backward(createdDirs) {
				info, err := target.Lstat(dir)
				if err == nil && info.IsDir() {
					err = target.Remove(dir)
				}
				// A player file may now occupy the directory. Leave it in place.
				if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, os.ErrExist) {
					rollback = errors.Join(rollback, err)
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
		if step.opposite != "" {
			if _, err := target.Lstat(step.opposite); !errors.Is(err, os.ErrNotExist) {
				if err != nil {
					return err
				}
				return i18n.Error("modpack.preview_changed")
			}
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
		if err := makeUpdateDirs(target, dir, &createdDirs); err != nil {
			return err
		}
		parent, err := target.OpenRoot(dir)
		if err != nil {
			return err
		}
		err = storage.PublishNew(ctx, parent, path.Base(step.Path), 0644, func(f *os.File) error {
			src, err := stage.Open(step.source)
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

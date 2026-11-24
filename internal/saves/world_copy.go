package saves

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

// WorldCopyCompatible checks registered instance identity and version metadata.
// PreviewWorldCopy also checks the resolved directories.
func WorldCopyCompatible(source, target instance.Instance) bool {
	if source.ID <= 0 || target.ID <= 0 || source.ID == target.ID || source.Version == "" || source.Version != target.Version ||
		!filepath.IsAbs(source.GameDir) || !filepath.IsAbs(target.GameDir) || directoryContains(source.GameDir, target.GameDir) ||
		directoryContains(target.GameDir, source.GameDir) {
		return false
	}
	if source.Vanilla() || target.Vanilla() {
		return source.Vanilla() && target.Vanilla()
	}
	return (source.UsesKnot() || source.UsesInstaller()) && strings.EqualFold(source.Loader, target.Loader) &&
		source.LoaderVersion != "" && source.LoaderVersion == target.LoaderVersion
}

func directoryContains(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && filepath.IsLocal(relative)
}

type WorldCopyPlan struct {
	source, target instance.Instance
	world          World
	destination    string
	targetDir      fs.FileInfo
	targetSaves    fs.FileInfo
	files          []backupFile
}

func (p *WorldCopyPlan) Destination() string {
	return filepath.Join(p.target.GameDir, "saves", p.destination)
}

// PreviewWorldCopy binds the listed source world and first free target name
// without creating the target saves directory.
func PreviewWorldCopy(ctx context.Context, source, target instance.Instance, world World) (*WorldCopyPlan, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !WorldCopyCompatible(source, target) {
		return nil, i18n.Error("instance.duplicate_target_changed")
	}
	src, err := savesRoot(source.GameDir, false)
	if err != nil {
		return nil, err
	}
	defer src.Close()
	files, err := worldCopySnapshots(ctx, src, world)
	if err != nil {
		return nil, err
	}
	base, err := distinctWorldCopyTarget(source.GameDir, target.GameDir)
	if err != nil {
		return nil, err
	}
	defer base.Close()
	dir, err := base.Stat(".")
	if err != nil {
		return nil, err
	}
	p := &WorldCopyPlan{source: source, target: target, world: world, destination: world.Folder(), targetDir: dir, files: files}
	if !storage.ValidFilename(p.destination) {
		return nil, i18n.Error("saves.path_invalid", p.destination)
	}
	destination, err := openBackupDirectory(base, "saves")
	if errors.Is(err, fs.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return nil, err
	}
	defer destination.Close()
	p.targetSaves, err = destination.Stat(".")
	if err != nil {
		return nil, err
	}
	srcDir, err := src.Stat(".")
	if err != nil {
		return nil, err
	}
	if os.SameFile(srcDir, p.targetSaves) {
		return nil, i18n.Error("instance.duplicate_target_changed")
	}
	for suffix := 2; ; suffix++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		_, err := destination.Lstat(p.destination)
		if errors.Is(err, fs.ErrNotExist) {
			return p, nil
		}
		if err != nil {
			return nil, err
		}
		p.destination = fmt.Sprintf("%s (%d)", world.Folder(), suffix)
		if !storage.ValidFilename(p.destination) {
			return nil, i18n.Error("saves.path_invalid", p.destination)
		}
	}
}

func distinctWorldCopyTarget(source, target string) (*os.Root, error) {
	sourcePath, err := filepath.EvalSymlinks(source)
	if err != nil {
		return nil, err
	}
	targetPath, err := filepath.EvalSymlinks(target)
	if err != nil {
		return nil, err
	}
	if directoryContains(sourcePath, targetPath) || directoryContains(targetPath, sourcePath) {
		return nil, i18n.Error("instance.duplicate_nested_directory")
	}
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		return nil, err
	}
	result, err := os.OpenRoot(targetPath)
	if err != nil {
		return nil, err
	}
	targetInfo, err := result.Stat(".")
	if err == nil && !os.SameFile(sourceInfo, targetInfo) {
		return result, nil
	}
	result.Close()
	if err != nil {
		return nil, err
	}
	return nil, i18n.Error("instance.duplicate_target_changed")
}

func worldCopySnapshots(ctx context.Context, root *os.Root, world World) ([]backupFile, error) {
	files, err := planBackup(ctx, root, &world)
	if err != nil {
		return nil, err
	}
	for n, file := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// Scoped Lstat captures the file ID eagerly on Windows.
		info, err := root.Lstat(filepath.FromSlash(file.name))
		if err != nil {
			return nil, err
		}
		if !sameBackupFile(file.info, info) {
			return nil, i18n.Error("saves.backup_file_changed", file.name)
		}
		files[n].info = info
	}
	return files, nil
}

func (p *WorldCopyPlan) verifySource(ctx context.Context, src *os.Root) error {
	current, err := worldCopySnapshots(ctx, src, p.world)
	if err != nil {
		return err
	}
	if !slices.EqualFunc(p.files, current, func(a, b backupFile) bool {
		return a.name == b.name && sameBackupFile(a.info, b.info)
	}) {
		return i18n.Error("saves.world_changed", p.world.Folder())
	}
	return nil
}

// Apply writes the selected world to the exact reserved name. The new world
// is published by the last step, which makes level.dat visible.
func (p *WorldCopyPlan) Apply(ctx context.Context) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	src, err := savesRoot(p.source.GameDir, false)
	if err != nil {
		return err
	}
	defer src.Close()
	if err := p.verifySource(ctx, src); err != nil {
		return err
	}
	base, err := distinctWorldCopyTarget(p.source.GameDir, p.target.GameDir)
	if err != nil {
		return err
	}
	defer base.Close()
	targetInfo, err := base.Stat(".")
	if err != nil {
		return err
	}
	if !os.SameFile(targetInfo, p.targetDir) {
		return i18n.Error("instance.duplicate_target_changed")
	}
	targetSaves, created, err := p.openTargetSaves(base)
	if err != nil {
		return err
	}
	if created != nil {
		defer func() {
			if err != nil {
				err = errors.Join(err, removeCopyPath(base, "saves", created))
			}
		}()
	}
	defer targetSaves.Close()
	srcDir, err := src.Stat(".")
	if err != nil {
		return err
	}
	savesDir, err := targetSaves.Stat(".")
	if err != nil {
		return err
	}
	if os.SameFile(srcDir, savesDir) {
		return i18n.Error("instance.duplicate_target_changed")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := targetSaves.Mkdir(p.destination, 0700); err != nil {
		return err
	}
	info, err := targetSaves.Lstat(p.destination)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, removeCopyPath(targetSaves, p.destination, info))
		}
	}()
	reserved, err := openBackupDirectory(targetSaves, p.destination)
	if err != nil {
		return err
	}
	defer reserved.Close()
	current, err := reserved.Stat(".")
	if err != nil {
		return err
	}
	if !os.SameFile(current, info) {
		return i18n.Error("instance.duplicate_target_changed")
	}
	output := copyOutput{root: reserved, directories: make(map[string]bool)}
	defer func() {
		if err != nil {
			err = errors.Join(err, output.rollback())
		}
	}()

	var level backupFile
	for _, file := range p.files {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, relative, _ := strings.Cut(file.name, "/")
		if relative == "level.dat" {
			level = file
			continue
		}
		if err := output.write(ctx, src, filepath.FromSlash(relative), file); err != nil {
			return err
		}
	}
	return storage.PublishNew(ctx, reserved, "level.dat", 0600, func(out *os.File) error {
		if err := copyWorldSnapshot(ctx, src, level, out); err != nil {
			return err
		}
		if err := p.verifySource(ctx, src); err != nil {
			return err
		}
		if err := p.world.Check(p.source.GameDir); err != nil {
			return err
		}
		current, err := os.Stat(p.target.GameDir)
		if err != nil {
			return err
		}
		if !os.SameFile(current, p.targetDir) || !sameCopyPath(base, "saves", savesDir) || !sameCopyPath(targetSaves, p.destination, info) {
			return i18n.Error("instance.duplicate_target_changed")
		}
		return nil
	})
}

func (p *WorldCopyPlan) openTargetSaves(base *os.Root) (*os.Root, fs.FileInfo, error) {
	result, err := openBackupDirectory(base, "saves")
	if errors.Is(err, fs.ErrNotExist) && p.targetSaves == nil {
		createErr := base.Mkdir("saves", 0755)
		if createErr != nil && !errors.Is(createErr, fs.ErrExist) {
			return nil, nil, createErr
		}
		result, err = openBackupDirectory(base, "saves")
		if err != nil || createErr != nil {
			return result, nil, err
		}
		created, err := result.Stat(".")
		if err != nil {
			result.Close()
			return nil, nil, err
		}
		return result, created, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if p.targetSaves != nil {
		info, err := result.Stat(".")
		if err != nil || !os.SameFile(info, p.targetSaves) {
			result.Close()
			if err != nil {
				return nil, nil, err
			}
			return nil, nil, i18n.Error("instance.duplicate_target_changed")
		}
	}
	return result, nil, nil
}

type copyPath struct {
	name string
	info fs.FileInfo
}

type copyOutput struct {
	root        *os.Root
	directories map[string]bool
	created     []copyPath
}

func (o *copyOutput) makeDirectory(name string) error {
	if name == "." || o.directories[name] {
		return nil
	}
	if err := o.makeDirectory(filepath.Dir(name)); err != nil {
		return err
	}
	if err := o.root.Mkdir(name, 0700); err != nil {
		return err
	}
	info, err := o.root.Lstat(name)
	if err != nil {
		return err
	}
	if info.Mode().Type() != fs.ModeDir || isBackupReparsePoint(info) {
		return i18n.Error("saves.path_invalid", name)
	}
	o.directories[name] = true
	o.created = append(o.created, copyPath{name, info})
	return nil
}

func (o *copyOutput) write(ctx context.Context, source *os.Root, name string, file backupFile) error {
	if err := o.makeDirectory(filepath.Dir(name)); err != nil {
		return err
	}
	out, err := o.root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	info, err := out.Stat()
	if err != nil {
		return errors.Join(err, out.Close())
	}
	o.created = append(o.created, copyPath{name, info})
	return errors.Join(copyWorldSnapshot(ctx, source, file, out), out.Sync(), out.Close())
}

func (o *copyOutput) rollback() error {
	var err error
	for _, entry := range slices.Backward(o.created) {
		err = errors.Join(err, removeCopyPath(o.root, entry.name, entry.info))
	}
	return err
}

func sameCopyPath(root *os.Root, name string, expected fs.FileInfo) bool {
	current, err := root.Lstat(name)
	return err == nil && current.Mode().Type() == expected.Mode().Type() && !isBackupReparsePoint(current) && os.SameFile(current, expected)
}

func removeCopyPath(root *os.Root, name string, expected fs.FileInfo) error {
	if sameCopyPath(root, name, expected) {
		// Remove is non-recursive, so it cannot remove another writer's files.
		if err := root.Remove(name); !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

func copyWorldSnapshot(ctx context.Context, root *os.Root, file backupFile, out *os.File) error {
	in, err := root.Open(filepath.FromSlash(file.name))
	if err != nil {
		return err
	}
	defer in.Close()
	before, err := in.Stat()
	if err != nil {
		return err
	}
	if !sameBackupFile(file.info, before) {
		return i18n.Error("saves.backup_file_changed", file.name)
	}
	if _, err := io.CopyN(out, transfer.Reader(ctx, in), before.Size()); err != nil {
		return err
	}
	after, err := in.Stat()
	if err != nil {
		return err
	}
	if !sameBackupFile(before, after) {
		return i18n.Error("saves.backup_file_changed", file.name)
	}
	return nil
}

package content

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

const packCopyChanged = "resource.pack_copy_changed"

type packCopyRoots struct {
	source, target           fs.FileInfo
	sourcePacks, targetPacks fs.FileInfo
}

// PackCopyTarget matches the exact game version. Resource packs are independent
// of the mod loader and retain only the launcher's file-based enabled state.
func PackCopyTarget(source, target instance.Instance) bool {
	return source.ID != target.ID && source.Version == target.Version &&
		ResourcePacks.Supports(source) && ResourcePacks.Supports(target) && filepath.Clean(source.GameDir) != filepath.Clean(target.GameDir)
}

func openPackCopyGame(i instance.Instance, expected fs.FileInfo) (*os.Root, fs.FileInfo, error) {
	name := filepath.Clean(i.GameDir)
	info, err := os.Lstat(name)
	if err != nil {
		return nil, nil, err
	}
	return openPackCopyRoot(info, expected, func() (*os.Root, error) { return os.OpenRoot(name) })
}

func openPackCopyRoot(entry, expected fs.FileInfo, open func() (*os.Root, error)) (*os.Root, fs.FileInfo, error) {
	if !entry.IsDir() {
		return nil, nil, i18n.Error("resource.pack_copy_directory")
	}
	root, err := open()
	if err != nil {
		return nil, nil, err
	}
	current, err := root.Stat(".")
	if err != nil {
		root.Close()
		return nil, nil, err
	}
	if !os.SameFile(entry, current) || expected != nil && !os.SameFile(expected, current) {
		root.Close()
		return nil, nil, i18n.Error(packCopyChanged)
	}
	return root, current, nil
}

// Open only a plain resourcepacks directory under the already verified game root.
func openPackCopyDirectory(game *os.Root, create bool) (*os.Root, fs.FileInfo, error) {
	name := string(ResourcePacks)
	if create {
		if err := game.Mkdir(name, 0755); err != nil {
			return nil, nil, err
		}
	}
	entry, err := game.Lstat(name)
	if err != nil {
		return nil, nil, err
	}
	return openPackCopyRoot(entry, nil, func() (*os.Root, error) { return game.OpenRoot(name) })
}

func packCopyNames(ctx context.Context, root *os.Root) (map[string]int, error) {
	names := make(map[string]int)
	if root == nil {
		return names, nil
	}
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		names[strings.TrimSuffix(strings.ToLower(entry.Name()), ".disabled")]++
	}
	return names, nil
}

func openSelectedPack(root *os.Root, file File) (*os.File, error) {
	name := file.DiskName()
	entry, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !entry.Mode().IsRegular() || entry.Size() != file.Size || !entry.ModTime().Equal(file.Modified) {
		return nil, i18n.Error(packCopyChanged)
	}
	in, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	current, err := in.Stat()
	if err != nil {
		in.Close()
		return nil, err
	}
	if !os.SameFile(entry, current) || current.Size() != file.Size || !current.ModTime().Equal(file.Modified) {
		in.Close()
		return nil, i18n.Error(packCopyChanged)
	}
	return in, nil
}

func previewSelectedPack(ctx context.Context, root *os.Root, file File) (string, error) {
	in, err := openSelectedPack(root, file)
	if err != nil {
		return "", err
	}
	defer in.Close()
	z, err := zip.NewReader(in, file.Size)
	if err != nil {
		return "", i18n.Error("resource.archive_invalid", err)
	}
	index, err := storage.IndexPackEntries(z.File, MaxSize)
	if err != nil {
		return "", err
	}
	if index["pack.mcmeta"] == nil {
		return "", i18n.Error("resource.archive_missing_marker", "pack.mcmeta")
	}
	return copyDigest(ctx, in, file.Size, packCopyChanged)
}

// PreviewPackCopy captures only selected ZIPs and creates no target files.
func PreviewPackCopy(ctx context.Context, source, target instance.Instance, selected []File) (*CopyPlan, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !PackCopyTarget(source, target) {
		return nil, i18n.Error("resource.pack_copy_target")
	}
	if len(selected) > storage.MaxPackEntries {
		return nil, i18n.Error("resource.pack_copy_budget")
	}
	if err := CheckSeparateDirectory(source.GameDir, target.GameDir); err != nil {
		return nil, err
	}
	srcGame, srcInfo, err := openPackCopyGame(source, nil)
	if err != nil {
		return nil, err
	}
	defer srcGame.Close()
	dstGame, dstInfo, err := openPackCopyGame(target, nil)
	if err != nil {
		return nil, err
	}
	defer dstGame.Close()
	if os.SameFile(srcInfo, dstInfo) {
		return nil, i18n.Error("resource.pack_copy_target")
	}
	src, srcPacks, err := openPackCopyDirectory(srcGame, false)
	if err != nil {
		return nil, err
	}
	defer src.Close()
	dst, dstPacks, err := openPackCopyDirectory(dstGame, false)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if dst != nil {
		defer dst.Close()
	}
	existing, err := packCopyNames(ctx, dst)
	if err != nil {
		return nil, err
	}
	chosen := make(map[string]int, len(selected))
	for _, file := range selected {
		chosen[strings.ToLower(file.Name)]++
	}
	p := &CopyPlan{source: source, target: target, packRoots: &packCopyRoots{
		source: srcInfo, target: dstInfo, sourcePacks: srcPacks, targetPacks: dstPacks,
	}}
	var total int64
	for _, file := range selected {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if file.Directory {
			return nil, i18n.Error("resource.filename_or_size_invalid")
		}
		if err := validateResource(source, ResourcePacks, file.Name, file.Size); err != nil {
			return nil, err
		}
		item := CopyItem{File: file}
		key := strings.ToLower(file.Name)
		if existing[key] > 0 || chosen[key] > 1 {
			item.Conflict = i18n.Text("resource.copy_name_conflict", file.Name)
		} else {
			if file.Size > storage.MaxPackBytes-total {
				return nil, i18n.Error("resource.pack_copy_budget")
			}
			total += file.Size
			item.digest, err = previewSelectedPack(ctx, src, file)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", file.Name, err)
			}
			p.Ready++
		}
		p.Items = append(p.Items, item)
	}
	return p, nil
}

func (p *CopyPlan) applyPacks(ctx context.Context) (int, error) {
	if err := ctx.Err(); err != nil || p.Ready == 0 {
		return 0, err
	}
	if err := CheckSeparateDirectory(p.source.GameDir, p.target.GameDir); err != nil {
		return 0, err
	}
	srcGame, _, err := openPackCopyGame(p.source, p.packRoots.source)
	if err != nil {
		return 0, err
	}
	defer srcGame.Close()
	dstGame, _, err := openPackCopyGame(p.target, p.packRoots.target)
	if err != nil {
		return 0, err
	}
	defer dstGame.Close()
	src, srcPacks, err := openPackCopyDirectory(srcGame, false)
	if err != nil {
		return 0, err
	}
	defer src.Close()
	if !os.SameFile(p.packRoots.sourcePacks, srcPacks) {
		return 0, i18n.Error(packCopyChanged)
	}
	dst, dstPacks, err := openPackCopyDirectory(dstGame, false)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return 0, err
	}
	defer func() {
		if dst != nil {
			dst.Close()
		}
	}()
	if (p.packRoots.targetPacks != nil || dstPacks != nil) && !os.SameFile(p.packRoots.targetPacks, dstPacks) {
		return 0, i18n.Error(packCopyChanged)
	}
	existing, err := packCopyNames(ctx, dst)
	if err != nil {
		return 0, err
	}
	for _, item := range p.Items {
		if item.Conflict == "" && existing[strings.ToLower(item.File.Name)] != 0 {
			return 0, i18n.Error(packCopyChanged)
		}
	}
	done := 0
	for _, item := range p.Items {
		if item.Conflict != "" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return done, err
		}
		in, err := openSelectedPack(src, item.File)
		if err != nil {
			return done, fmt.Errorf("%s: %w", item.File.Name, err)
		}
		if dst == nil && ctx.Err() == nil {
			dst, dstPacks, err = openPackCopyDirectory(dstGame, true)
		}
		if err == nil {
			err = ctx.Err()
		}
		if err == nil {
			write := verifiedResourceWriter(ctx, in, item.File.Size, sha256.New(), item.digest)
			err = publishResourceIn(ctx, dst, p.target, ResourcePacks, item.File, func(out *os.File) error {
				if err := write(out); err != nil {
					return err
				}
				game, gameErr := os.Lstat(filepath.Clean(p.target.GameDir))
				dir, dirErr := dstGame.Lstat(string(ResourcePacks))
				if gameErr != nil || dirErr != nil || !game.IsDir() || !dir.IsDir() ||
					!os.SameFile(p.packRoots.target, game) || !os.SameFile(dstPacks, dir) {
					return i18n.Error(packCopyChanged)
				}
				return nil
			})
		}
		in.Close()
		if err != nil {
			return done, fmt.Errorf("%s: %w", item.File.Name, err)
		}
		done++
	}
	return done, nil
}

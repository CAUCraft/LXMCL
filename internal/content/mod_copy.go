package content

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

// ModCopyTarget matches the game version and loader family used by the local inventory.
func ModCopyTarget(source, target instance.Instance) bool {
	return source.ID != target.ID && source.Version == target.Version && strings.EqualFold(source.Loader, target.Loader) &&
		Mods.Supports(source) && Mods.Supports(target) && filepath.Clean(source.GameDir) != filepath.Clean(target.GameDir)
}

type CopyItem struct {
	File     File
	Conflict string
	digest   string
}

type CopyPlan struct {
	source, target instance.Instance
	packRoots      *packCopyRoots
	Items          []CopyItem
	Ready          int
}

type modCopyIndex struct {
	names, ids map[string]int
}

func indexModFiles(files []File) modCopyIndex {
	index := modCopyIndex{map[string]int{}, map[string]int{}}
	for _, file := range files {
		index.names[strings.ToLower(file.Name)]++
		for _, mod := range file.mods {
			index.ids[mod.id]++
		}
	}
	return index
}

func (index modCopyIndex) conflict(f File, limit int) string {
	if index.names[strings.ToLower(f.Name)] > limit {
		return i18n.Text("resource.copy_name_conflict", f.Name)
	}
	for _, mod := range f.mods {
		if index.ids[mod.id] > limit {
			return i18n.Text("resource.copy_id_conflict", mod.id)
		}
	}
	return ""
}

// PreviewModCopy does not create the target mods directory. Only explicitly
// selected files can enter the plan, including their physical enabled state.
func PreviewModCopy(ctx context.Context, source, target instance.Instance, selected []File) (*CopyPlan, error) {
	if !ModCopyTarget(source, target) {
		return nil, i18n.Error("resource.copy_target")
	}
	if err := CheckSeparateDirectory(source.GameDir, target.GameDir); err != nil {
		return nil, err
	}
	files, err := Inventory(ctx, target, Mods)
	if err != nil {
		return nil, err
	}
	src, err := OpenDirectory(source, Mods, false)
	if err != nil {
		return nil, err
	}
	defer src.Close()
	chosen, existing := indexModFiles(selected), indexModFiles(files)
	p := &CopyPlan{source: source, target: target}
	for _, f := range selected {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		item := CopyItem{File: f, Conflict: existing.conflict(f, 0)}
		if item.Conflict == "" {
			item.Conflict = chosen.conflict(f, 1)
		}
		if item.Conflict == "" {
			item.digest, err = selectedModDigest(ctx, src, source, f)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", f.Name, err)
			}
			p.Ready++
		}
		p.Items = append(p.Items, item)
	}
	return p, nil
}

func selectedModDigest(ctx context.Context, root *os.Root, source instance.Instance, f File) (string, error) {
	if err := validateResource(source, Mods, f.Name, f.Size); err != nil {
		return "", err
	}
	file, err := root.Open(f.DiskName())
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() != f.Size || !info.ModTime().Equal(f.Modified) {
		return "", i18n.Error("resource.copy_changed")
	}
	_, markers, _ := Mods.Format(source)
	_, ids := modLabels(manifestMods(file, f.Size, markers))
	if ids != f.ModIDs {
		return "", i18n.Error("resource.copy_changed")
	}
	return copyDigest(ctx, file, f.Size, "resource.copy_changed")
}

func copyDigest(ctx context.Context, file io.Reader, size int64, changed string) (string, error) {
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(transfer.Reader(ctx, file), size+1))
	if err != nil {
		return "", err
	}
	if n != size {
		return "", i18n.Error(changed)
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// Apply rechecks the destination and publishes only approved files. As with a
// batch import, a failure retains earlier completed copies and returns their count.
func (p *CopyPlan) Apply(ctx context.Context) (int, error) {
	if p.packRoots != nil {
		return p.applyPacks(ctx)
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := CheckSeparateDirectory(p.source.GameDir, p.target.GameDir); err != nil {
		return 0, err
	}
	files, err := Inventory(ctx, p.target, Mods)
	if err != nil {
		return 0, err
	}
	index := indexModFiles(files)
	for _, item := range p.Items {
		if item.Conflict == "" && index.conflict(item.File, 0) != "" {
			return 0, i18n.Error("resource.copy_changed")
		}
	}
	src, err := OpenDirectory(p.source, Mods, false)
	if err != nil {
		return 0, err
	}
	defer src.Close()
	done := 0
	for _, item := range p.Items {
		if item.Conflict != "" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return done, err
		}
		file, err := src.Open(item.File.DiskName())
		if err == nil {
			err = writeVerifiedFile(ctx, p.target, Mods, item.File, file, sha256.New(), item.digest)
			file.Close()
		}
		if err != nil {
			return done, fmt.Errorf("%s: %w", item.File.Name, err)
		}
		done++
	}
	return done, nil
}

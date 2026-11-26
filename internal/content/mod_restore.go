package content

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/CAUCraft/LXMCL/internal/tasks"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

type ModRestorePlan struct {
	target   instance.Instance
	filename string
	snapshot fs.FileInfo
	Items    []CopyItem
}

// withModBackup validates the whole archive before using its direct mods/ entries.
func withModBackup(ctx context.Context, filename string, use func(fs.FileInfo, map[string]*zip.File) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !filepath.IsAbs(filename) || !strings.EqualFold(filepath.Ext(filename), ".zip") {
		return i18n.Error("resource.restore_zip")
	}
	f, size, err := storage.OpenPackFile(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	z, err := zip.NewReader(f, size)
	if err != nil {
		return i18n.Error("resource.archive_invalid", err)
	}
	entries, err := storage.IndexPackEntries(z.File, MaxSize)
	if err != nil {
		return err
	}
	paths := make(storage.ArchivePaths)
	for _, e := range z.File {
		if err := paths.Add(strings.TrimSuffix(e.Name, "/"), e.FileInfo().IsDir()); err != nil {
			return err
		}
	}
	return use(info, entries)
}

// PreviewModRestore inspects local JARs with the target loader's manifest reader.
func PreviewModRestore(ctx context.Context, target instance.Instance, filename string) (*ModRestorePlan, error) {
	_, markers, err := Mods.Format(target)
	if err != nil {
		return nil, err
	}
	installed, err := Inventory(ctx, target, Mods)
	if err != nil {
		return nil, err
	}
	existing := indexModFiles(installed)
	p := &ModRestorePlan{target: target, filename: filename}
	err = withModBackup(ctx, filename, func(info fs.FileInfo, entries map[string]*zip.File) error {
		p.snapshot = info
		var recognized []File
		for _, entry := range slices.Sorted(maps.Keys(entries)) {
			if err := ctx.Err(); err != nil {
				return err
			}
			disk, inside := strings.CutPrefix(entry, "mods/")
			name, disabled := strings.CutSuffix(disk, ".disabled")
			if !inside || !ValidName(name, ".jar") {
				continue
			}
			item := CopyItem{File: File{Name: name, Enabled: !disabled, Size: int64(entries[entry].UncompressedSize64)}}
			if err := inspectBackupMod(ctx, entries[entry], markers, &item); err != nil {
				return fmt.Errorf("%s: %w", disk, err)
			}
			if item.Conflict == "" {
				recognized = append(recognized, item.File)
				item.Conflict = existing.conflict(item.File, 0)
			}
			p.Items = append(p.Items, item)
		}
		if len(p.Items) == 0 {
			return i18n.Error("resource.restore_zip")
		}
		inBackup := indexModFiles(recognized)
		for n := range p.Items {
			item := &p.Items[n]
			if item.Conflict != "" {
				continue
			}
			if conflict := inBackup.conflict(item.File, 1); conflict != "" {
				item.Conflict = i18n.Text("resource.restore_internal", conflict)
			}
		}
		return ctx.Err()
	})
	if err != nil {
		return nil, err
	}
	return p, nil
}

// A compressed JAR must be seekable for the manifest reader. Spool one bounded
// entry at a time without extracting into the instance or keeping it after preview.
func inspectBackupMod(ctx context.Context, e *zip.File, markers []string, item *CopyItem) error {
	src, err := e.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	tmp, err := os.CreateTemp("", "lxmcl-mod-preview-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(transfer.Reader(ctx, src), item.File.Size+1))
	if err != nil {
		return err
	}
	if n != item.File.Size {
		return i18n.Error("resource.checksum_failed")
	}
	item.File.mods = manifestMods(tmp, n, markers)
	if len(item.File.mods) == 0 {
		item.Conflict = i18n.Text("resource.restore_manifest")
		return nil
	}
	item.File.ModTitle, item.File.ModIDs = modLabels(item.File.mods)
	item.digest = fmt.Sprintf("%x", h.Sum(nil))
	return ctx.Err()
}

// Restore rechecks the target and verified bytes before publishing each chosen
// file. Completed files remain installed after a later failure or cancellation.
func (p *ModRestorePlan) Restore(ctx context.Context, selected map[string]bool, report func(tasks.Progress)) (restored, skipped int, err error) {
	if len(selected) == 0 {
		return 0, 0, i18n.Error("resource.backup_select")
	}
	err = withModBackup(ctx, p.filename, func(info fs.FileInfo, entries map[string]*zip.File) error {
		if !os.SameFile(info, p.snapshot) || info.Size() != p.snapshot.Size() || !info.ModTime().Equal(p.snapshot.ModTime()) {
			return i18n.Error("resource.restore_changed")
		}
		installed, err := Inventory(ctx, p.target, Mods)
		if err != nil {
			return err
		}
		index := indexModFiles(installed)
		for _, item := range p.Items {
			if err := ctx.Err(); err != nil {
				return err
			}
			disk := item.File.DiskName()
			if !selected[disk] {
				continue
			}
			if item.Conflict != "" || index.conflict(item.File, 0) != "" {
				skipped++
				continue
			}
			if report != nil {
				report(tasks.Progress{Stage: i18n.Text("progress.copy_instance_file", disk), Done: restored + skipped, Total: len(selected)})
			}
			entry := entries["mods/"+disk]
			if entry == nil || entry.UncompressedSize64 != uint64(item.File.Size) {
				return i18n.Error("resource.restore_changed")
			}
			src, err := entry.Open()
			if err != nil {
				return err
			}
			err = writeVerifiedFile(ctx, p.target, Mods, item.File, src, sha256.New(), item.digest)
			src.Close()
			if err != nil {
				return fmt.Errorf("%s: %w", disk, err)
			}
			restored++
		}
		return nil
	})
	return restored, skipped, err
}

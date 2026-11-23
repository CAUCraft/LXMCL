package content

import (
	"archive/zip"
	"context"
	"os"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/CAUCraft/LXMCL/internal/tasks"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

// BackupMods publishes the captured files under mods/, using their physical enabled or disabled names.
func BackupMods(ctx context.Context, source instance.Instance, selected []File, filename string, report func(tasks.Progress)) error {
	if len(selected) == 0 {
		return i18n.Error("resource.backup_select")
	}
	src, err := OpenDirectory(source, Mods, false)
	if err != nil {
		return err
	}
	defer src.Close()
	return publishInstanceZIP(ctx, source.GameDir, filename, func(dst *os.File) error {
		z := zip.NewWriter(dst)
		paths := make(storage.ArchivePaths)
		for index, file := range selected {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := validateResource(source, Mods, file.Name, file.Size); err != nil {
				return err
			}
			name := file.DiskName()
			entry := "mods/" + name
			if err := paths.Add(entry, false); err != nil {
				return err
			}
			st, err := src.Lstat(name)
			if err != nil {
				return err
			}
			if !st.Mode().IsRegular() || st.Size() != file.Size || !st.ModTime().Equal(file.Modified) {
				return i18n.Error("resource.backup_changed", name)
			}
			if report != nil {
				report(tasks.Progress{Stage: i18n.Text("progress.copy_instance_file", name), Done: index, Total: len(selected)})
			}
			if err := transfer.WriteZIPFile(ctx, src, z, name, entry, st); err != nil {
				return err
			}
		}
		return z.Close()
	})
}

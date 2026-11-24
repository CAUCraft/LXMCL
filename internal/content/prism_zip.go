package content

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/distribution"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/CAUCraft/LXMCL/internal/tasks"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

// ReadPrismZIP previews metadata without extracting to disk.
func ReadPrismZIP(ctx context.Context, filename string) (PrismInstance, error) {
	var preview PrismInstance
	err := withPrismZIP(ctx, filename, func(p PrismInstance, z *zip.Reader) error {
		if p.Worlds {
			folders := make(map[string]struct{})
			prefix := path.Join(p.archiveRoot, p.GameFolder, "saves") + "/"
			for _, entry := range z.File {
				if err := ctx.Err(); err != nil {
					return err
				}
				if name, inSaves := strings.CutPrefix(entry.Name, prefix); inSaves {
					if folder, _, inFolder := strings.Cut(name, "/"); inFolder {
						folders[folder] = struct{}{}
					}
				}
			}
			names := slices.Sorted(maps.Keys(folders))
			p.worldFolders = &names
		}
		preview = p
		return ctx.Err()
	})
	return preview, err
}

// Keep the inspected archive open through extraction so replacement of the source
// path cannot redirect the import after the preview is rechecked.
func withPrismZIP(ctx context.Context, filename string, use func(PrismInstance, *zip.Reader) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !filepath.IsAbs(filename) || !strings.EqualFold(filepath.Ext(filename), ".zip") {
		return i18n.Error("prism.zip_layout")
	}
	f, size, err := storage.OpenPackFile(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	z, err := zip.NewReader(f, size)
	if err != nil {
		return i18n.Error("resource.archive_invalid", err)
	}
	files, err := storage.IndexPackEntries(z.File, distribution.MaxFileSize)
	if err != nil {
		return err
	}
	paths := make(storage.ArchivePaths)
	index := sha256.New()
	var folder string
	for _, entry := range z.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := strings.TrimSuffix(entry.Name, "/")
		dir := entry.Mode().IsDir()
		if err := paths.Add(name, dir); err != nil {
			return err
		}
		fmt.Fprintf(index, "%q %t %d %08x\n", name, dir, entry.UncompressedSize64, entry.CRC32)
		if dir || path.Base(name) != "instance.cfg" || strings.Count(name, "/") > 1 {
			continue
		}
		parent := path.Dir(name)
		if files[path.Join(parent, "mmc-pack.json")] == nil {
			continue
		}
		if folder != "" {
			return i18n.Error("prism.zip_layout")
		}
		folder = parent
	}
	if folder == "" {
		return i18n.Error("prism.zip_layout")
	}
	root, err := fs.Sub(z, folder)
	if err != nil {
		return err
	}
	p := PrismInstance{Source: filepath.Clean(filename), archiveRoot: folder}
	copy(p.archiveIndex[:], index.Sum(nil))
	fallback := folder
	if folder == "." {
		fallback = strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	}
	p, err = readPrismProfile(ctx, root, p, fallback)
	if err != nil {
		return err
	}
	return use(p, z)
}

func copyPrismZIP(ctx context.Context, p PrismInstance, destination string, selected PrismFiles, commit func() error, report func(tasks.Progress)) error {
	return withPrismZIP(ctx, p.Source, func(now PrismInstance, z *zip.Reader) error {
		if now != p {
			return i18n.Error("prism.changed")
		}
		game := path.Join(p.archiveRoot, p.GameFolder) + "/"
		return stageNewDirectory(ctx, p.Source, destination, commit, report, func(dst *os.Root) error {
			done := 0
			for _, entry := range z.File {
				if err := ctx.Err(); err != nil {
					return err
				}
				name, inGame := strings.CutPrefix(entry.Name, game)
				includeWorld := selected.Worlds && strings.HasPrefix(name, "saves/")
				includeOptions := selected.Options && name == "options.txt"
				includeServers := selected.Servers && name == serverListPath.name
				if !inGame || (!includeWorld && !includeOptions && !includeServers) {
					continue
				}
				name = strings.TrimSuffix(name, "/")
				if report != nil {
					report(tasks.Progress{Stage: i18n.Text("progress.copy_instance_file", name), Done: done})
				}
				if err := transfer.ExtractZIPEntry(ctx, dst, entry, name); err != nil {
					return err
				}
				done++
			}
			return nil
		})
	})
}

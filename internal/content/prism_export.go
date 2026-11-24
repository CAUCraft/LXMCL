package content

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/distribution"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/nbt"
	"github.com/CAUCraft/LXMCL/internal/saves"
	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/CAUCraft/LXMCL/internal/tasks"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

// MultiMC's line-based settings format is also supported by Prism.
var escapePrismName = strings.NewReplacer("\\", "\\\\", "\n", "\\n", "\t", "\\t", "#", "\\#")
var unescapePrismName = strings.NewReplacer("\\\\", "\\", "\\n", "\n", "\\t", "\t", "\\#", "#")

// ExportPrismZIP publishes a new archive containing only portable vanilla configuration
// and the optional game data selected by the player.
func ExportPrismZIP(ctx context.Context, i instance.Instance, filename string, worlds, options bool, report func(tasks.Progress)) error {
	return ExportPrismFiles(ctx, i, filename, PrismFiles{Worlds: worlds, Options: options}, report)
}

// ExportPrismFiles writes the selected game files to a new Prism or MultiMC ZIP.
func ExportPrismFiles(ctx context.Context, i instance.Instance, filename string, selected PrismFiles, report func(tasks.Progress)) error {
	if selected.WorldFolders != nil {
		return ErrPrismExportWorldsChanged
	}
	return exportPrismFiles(ctx, i, filename, selected, nil, nil, report)
}

type prismExportAdd func(root *os.Root, source, relative string, info fs.FileInfo) error

// ExportPrismTaskZIP also records the exact published ZIP when its identity is available.
// A nil selection keeps the existing all-or-no-worlds behavior.
func ExportPrismTaskZIP(ctx context.Context, i instance.Instance, filename string, files PrismFiles, preview *PrismExportWorlds, selected []string, report func(tasks.Progress)) (tasks.PrismExport, error) {
	var artifact tasks.PrismExport
	var selectedWorlds func(prismExportAdd) error
	if files.WorldFolders != nil {
		return artifact, ErrPrismExportWorldsChanged
	}
	if selected != nil {
		var err error
		selectedWorlds, err = prismSelectedWorldWriter(ctx, i, files, preview, selected)
		if err != nil {
			return artifact, err
		}
	}
	err := exportPrismFiles(ctx, i, filename, files, selectedWorlds, &artifact, report)
	return artifact, err
}

func exportPrismFiles(ctx context.Context, i instance.Instance, filename string, selected PrismFiles, selectedWorlds func(prismExportAdd) error, artifact *tasks.PrismExport, report func(tasks.Progress)) error {
	if err := i.Validate(); err != nil {
		return err
	}
	if !i.Vanilla() {
		return i18n.Error("prism.version")
	}
	portable := instance.LaunchOptions{Memory: i.Launch.Memory, Display: i.Launch.Display, Width: i.Launch.Width, Height: i.Launch.Height}
	if err := portable.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(i.Name) == "" || strings.ContainsAny(i.Name, "\x00\r") {
		return i18n.Error("prism.export_name")
	}
	prepare := func(f *os.File) error {
		z := zip.NewWriter(&limitedZIPWriter{file: f, remaining: storage.MaxPackSize})
		total, err := writePrismMetadata(z, i)
		if err != nil {
			return err
		}
		count := 2
		paths := make(storage.ArchivePaths)
		add := func(root *os.Root, source, relative string, info fs.FileInfo) error {
			entry := "instance/.minecraft/" + relative
			if err := paths.Add(entry, false); err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return i18n.Error("instance.duplicate_file_unsupported", source)
			}
			if count >= storage.MaxPackEntries || info.Size() > distribution.MaxFileSize || info.Size() > storage.MaxPackBytes-total {
				return i18n.Error("modpack.archive_expanded_too_large")
			}
			count++
			total += info.Size()
			if report != nil {
				report(tasks.Progress{Stage: i18n.Text("progress.copy_instance_file", relative), Done: count - 2})
			}
			return transfer.WriteZIPFile(ctx, root, z, source, entry, info)
		}
		if selected.Options {
			if err := addPrismOptions(i.GameDir, add); err != nil {
				return err
			}
		}
		if selected.Servers {
			if err := addPrismServerList(i.GameDir, add); err != nil {
				return i18n.Error("instance.duplicate_file_failed", serverListPath.name, err)
			}
		}
		if selectedWorlds != nil {
			if err := selectedWorlds(add); err != nil {
				return err
			}
		} else if selected.Worlds {
			err := saves.VisitWorldFiles(ctx, i.GameDir, func(root *os.Root, name string, info fs.FileInfo) error {
				_, relative, _ := strings.Cut(name, "/")
				if !prismWorldData(relative) {
					return nil
				}
				return add(root, name, "saves/"+name, info)
			})
			if err != nil {
				return err
			}
		}
		if err := z.Close(); err != nil {
			return err
		}
		return ctx.Err()
	}
	if artifact == nil {
		return publishInstanceZIP(ctx, i.GameDir, filename, prepare)
	}
	return withInstanceZIPDestination(i.GameDir, filename, func(out *os.Root, name string) error {
		stamp, err := storage.PublishNewStamped(ctx, out, name, 0600, prepare)
		if err == nil && stamp.Valid() && storage.CheckFileStamp(filename, stamp) == nil {
			*artifact = tasks.PrismExport{Path: filepath.Clean(filename), Stamp: stamp}
		}
		return err
	})
}

func writePrismMetadata(z *zip.Writer, i instance.Instance) (int64, error) {
	w, err := z.Create("instance/instance.cfg")
	if err != nil {
		return 0, err
	}
	cfg := "InstanceType=OneSix\nname=" + escapePrismName.Replace(i.Name) + "\n"
	if i.Launch.Memory != 0 {
		cfg += fmt.Sprintf("OverrideMemory=true\nMaxMemAlloc=%d\n", i.Launch.Memory)
	}
	if i.Launch.Display == instance.DisplayResolution {
		cfg += fmt.Sprintf("OverrideWindow=true\nLaunchMaximized=false\nMinecraftWinWidth=%d\nMinecraftWinHeight=%d\n", i.Launch.Width, i.Launch.Height)
	}
	if _, err := io.WriteString(w, cfg); err != nil {
		return 0, err
	}
	w, err = z.Create("instance/mmc-pack.json")
	if err != nil {
		return 0, err
	}
	component := map[string]any{"uid": "net.minecraft", "version": i.Version, "important": true}
	manifest, err := json.Marshal(map[string]any{"formatVersion": 1, "components": []map[string]any{component}})
	if err != nil {
		return 0, err
	}
	if _, err := w.Write(manifest); err != nil {
		return 0, err
	}
	return int64(len(cfg) + len(manifest)), nil
}

func addPrismOptions(gameDir string, add func(*os.Root, string, string, fs.FileInfo) error) error {
	root, err := os.OpenRoot(gameDir)
	if err != nil {
		return err
	}
	defer root.Close()
	info, err := root.Lstat("options.txt")
	if err != nil {
		return err
	}
	return add(root, "options.txt", "options.txt", info)
}

func addPrismServerList(gameDir string, add func(*os.Root, string, string, fs.FileInfo) error) error {
	root, err := os.OpenRoot(gameDir)
	if err != nil {
		return err
	}
	defer root.Close()
	name := serverListPath.name
	info, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if info.Size() > nbt.MaxBytes {
		return i18n.Error("server.game_list_large")
	}
	if err := add(root, name, name, info); err != nil {
		return err
	}
	if !currentPrismServerFile(root, info) {
		return i18n.Error("instance.duplicate_source_changed")
	}
	return nil
}

var playerProgressFile = regexp.MustCompile(`(?i)^[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}[.]json$`)

// Select game-owned world formats. Data packs, scripts, session locks, and
// unrelated files stored alongside a world are outside the export.
func prismWorldData(name string) bool {
	switch name {
	case "level.dat", "level.dat_old", "icon.png":
		return true
	}
	top, _, _ := strings.Cut(name, "/")
	ext := path.Ext(name)
	if !prismWorldDataFolder(top) {
		return false
	}
	switch path.Base(path.Dir(name)) {
	case "region", "poi", "entities":
		return ext == ".mca" || ext == ".mcc"
	}
	switch top {
	case "advancements", "stats", "players":
		if dir := path.Dir(name); dir == "advancements" || dir == "stats" || dir == "players/advancements" || dir == "players/stats" {
			return playerProgressFile.MatchString(path.Base(name))
		}
	}
	return ext == ".dat" || ext == ".dat_old" || ext == ".nbt"
}

func prismWorldDataFolder(name string) bool {
	switch name {
	case "region", "poi", "entities", "DIM-1", "DIM1", "dimensions", "data", "playerdata", "players", "generated", "advancements", "stats":
		return true
	default:
		return false
	}
}

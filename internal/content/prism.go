package content

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/nbt"
	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/CAUCraft/LXMCL/internal/tasks"
)

// PrismInstance describes only a vanilla profile and the data offered in the preview.
type PrismInstance struct {
	Source, GameFolder, Name, Version string
	Worlds, Options, Servers          bool
	Screenshots                       bool
	SkippedSettings                   bool
	Launch                            instance.LaunchOptions
	archiveRoot                       string // "." or the single wrapper folder in a ZIP; empty for a directory.
	archiveIndex                      [sha256.Size]byte
	serverList                        fs.FileInfo // Directory source identity. ZIP previews bind the archive index instead.
	worldFolders                      *prismWorldPreview
	directoryScreenshots              *prismDirectoryScreenshots
}

type prismWorldPreview struct {
	names []string
	saves fs.FileInfo
	dirs  map[string]fs.FileInfo
}

// ErrPrismWorldsChanged indicates that a selection cannot be imported from the accepted preview.
var ErrPrismWorldsChanged = i18n.Error("prism.worlds_changed")

// WorldFolders returns the names of direct directories in saves and whether the list is complete.
func (p PrismInstance) WorldFolders() ([]string, bool) {
	if p.worldFolders == nil {
		return nil, false
	}
	return slices.Clone(p.worldFolders.names), true
}

// PrismFiles identifies the game files selected for an import or export.
type PrismFiles struct {
	Worlds, Options, Servers bool
	Screenshots              bool
	WorldFolders             []string // For imports, nil uses Worlds as before. A non-nil slice selects exact folder names.
}

func ReadPrismInstance(ctx context.Context, dir string) (PrismInstance, error) {
	var p PrismInstance
	if !filepath.IsAbs(dir) {
		return p, i18n.Error("prism.select_root")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return p, err
	}
	defer root.Close()
	p, err = readPrismProfile(ctx, root.FS(), PrismInstance{Source: filepath.Clean(dir)}, filepath.Base(dir))
	if err == nil {
		if p.Worlds {
			p.worldFolders = readPrismWorldFolders(ctx, root, filepath.Join(p.GameFolder, "saves"))
		}
		p.directoryScreenshots = readPrismDirectoryScreenshots(ctx, root, p)
		p.Screenshots = p.directoryScreenshots != nil
		err = ctx.Err()
	}
	return p, err
}

func readPrismWorldFolders(ctx context.Context, root *os.Root, name string) *prismWorldPreview {
	dir, err := root.Open(name)
	if err != nil {
		return nil
	}
	defer dir.Close()
	before, err := dir.Stat()
	if err != nil || !before.IsDir() {
		return nil
	}
	preview := &prismWorldPreview{saves: before, dirs: make(map[string]fs.FileInfo)}
	count := 0
	for ctx.Err() == nil {
		entries, err := dir.ReadDir(min(128, storage.MaxPackEntries+1-count))
		count += len(entries)
		if count > storage.MaxPackEntries || (err != nil && !errors.Is(err, io.EOF)) {
			return nil
		}
		for _, entry := range entries {
			if !storage.ValidPortablePath("saves/"+entry.Name()) || (!entry.IsDir() && !entry.Type().IsRegular()) {
				return nil
			}
			if entry.IsDir() {
				// Windows directory listings do not always provide the identity used by SameFile.
				info, err := root.Lstat(filepath.Join(name, entry.Name()))
				if err != nil || !info.IsDir() {
					return nil
				}
				preview.dirs[entry.Name()] = info
			}
		}
		if errors.Is(err, io.EOF) {
			after, err := root.Lstat(name)
			if err != nil || !after.IsDir() || !os.SameFile(before, after) || !after.ModTime().Equal(before.ModTime()) {
				return nil
			}
			preview.names = slices.Sorted(maps.Keys(preview.dirs))
			return preview
		}
	}
	return nil
}

func readPrismProfile(ctx context.Context, root fs.FS, p PrismInstance, fallbackName string) (PrismInstance, error) {
	cfg, err := prismMetadata(ctx, root, "instance.cfg")
	if err != nil {
		return p, err
	}
	data, err := prismMetadata(ctx, root, "mmc-pack.json")
	if err != nil {
		return p, err
	}
	p.Version, err = prismVersion(root, data)
	if err != nil {
		return p, err
	}
	p.readSettings(cfg, fallbackName)
	for _, name := range []string{".minecraft", "minecraft"} {
		info, err := fs.Lstat(root, name)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return p, err
		}
		if !info.IsDir() || p.GameFolder != "" {
			return p, i18n.Error("prism.game_folder")
		}
		p.GameFolder = name
	}
	// A profile can exist before its first launch.
	if p.GameFolder == "" {
		return p, ctx.Err()
	}
	for _, item := range []struct {
		name    string
		dir     bool
		present *bool
	}{{"saves", true, &p.Worlds}, {"options.txt", false, &p.Options}} {
		info, err := fs.Lstat(root, path.Join(p.GameFolder, item.name))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return p, err
		}
		if (item.dir && !info.IsDir()) || (!item.dir && !info.Mode().IsRegular()) {
			return p, i18n.Error("instance.duplicate_file_unsupported", item.name)
		}
		*item.present = true
	}
	if info, err := fs.Lstat(root, path.Join(p.GameFolder, serverListPath.name)); err == nil && info.Mode().IsRegular() && info.Size() <= nbt.MaxBytes {
		p.Servers = true
		if p.archiveRoot == "" {
			p.serverList = info
		}
	}
	return p, ctx.Err()
}

func prismVersion(root fs.FS, data []byte) (string, error) {
	var version string
	var pack struct {
		FormatVersion int
		Components    []struct{ UID, Version string }
	}
	if json.Unmarshal(data, &pack) != nil || pack.FormatVersion != 1 {
		return "", i18n.Error("prism.format")
	}
	for _, c := range pack.Components {
		switch c.UID {
		case "net.minecraft":
			if version != "" || !instance.SupportedRelease(c.Version) {
				return "", i18n.Error("prism.version")
			}
			version = c.Version
		case "org.lwjgl3", "org.lwjgl":
		default:
			return "", i18n.Error("prism.component", c.UID)
		}
	}
	if version == "" {
		return "", i18n.Error("prism.version")
	}
	patches, err := fs.ReadDir(root, "patches")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	for _, e := range patches {
		if strings.EqualFold(filepath.Ext(e.Name()), ".json") {
			return "", i18n.Error("prism.component", e.Name())
		}
	}
	return version, nil
}

func (p *PrismInstance) readSettings(cfg []byte, fallbackName string) {
	fields := make(map[string]string)
	lines := strings.SplitSeq(strings.TrimPrefix(string(cfg), "\uFEFF"), "\n")
	general := true
	for line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			general = line == "[General]"
			continue
		}
		if key, value, ok := strings.Cut(line, "="); ok && general {
			key = strings.TrimSpace(key)
			switch key {
			case "name", "ConfigVersion", "OverrideMemory", "MaxMemAlloc", "OverrideWindow", "LaunchMaximized", "MinecraftWinWidth", "MinecraftWinHeight":
				fields[key] = strings.TrimSpace(value)
			}
		}
	}
	p.Name = fields["name"]
	if fields["ConfigVersion"] == "" {
		p.Name = unescapePrismName.Replace(p.Name)
	}
	if p.Name == "" {
		p.Name = fallbackName
	}
	if strings.EqualFold(fields["OverrideMemory"], "true") {
		mib, err := strconv.Atoi(fields["MaxMemAlloc"])
		if err == nil && instance.ValidateMemory(mib) == nil {
			p.Launch.Memory = mib
		} else {
			p.SkippedSettings = true
		}
	}
	if strings.EqualFold(fields["OverrideWindow"], "true") {
		width, _ := strconv.Atoi(fields["MinecraftWinWidth"])
		height, _ := strconv.Atoi(fields["MinecraftWinHeight"])
		custom := instance.LaunchOptions{Display: instance.DisplayResolution, Width: width, Height: height}
		if !strings.EqualFold(fields["LaunchMaximized"], "true") && custom.Validate() == nil {
			p.Launch.Display, p.Launch.Width, p.Launch.Height = custom.Display, width, height
		} else {
			p.SkippedSettings = true
		}
	}
}

func prismMetadata(ctx context.Context, root fs.FS, name string) ([]byte, error) {
	info, err := fs.Lstat(root, name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, i18n.Error("prism.select_root")
	}
	if err != nil {
		return nil, err
	}
	const limit = 1 << 20
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, i18n.Error("prism.format")
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, i18n.Error("prism.format")
	}
	return data, ctx.Err()
}

// CopyPrismInstance rechecks the accepted preview before publishing a new directory.
func CopyPrismInstance(ctx context.Context, p PrismInstance, destination string, worlds, options bool, commit func() error, report func(tasks.Progress)) error {
	return CopyPrismFiles(ctx, p, destination, PrismFiles{Worlds: worlds, Options: options}, commit, report)
}

// CopyPrismFiles publishes a new instance with exactly the chosen game files.
func CopyPrismFiles(ctx context.Context, p PrismInstance, destination string, selected PrismFiles, commit func() error, report func(tasks.Progress)) error {
	chosen, err := p.selectedWorldFolders(selected)
	if err != nil {
		return err
	}
	if (selected.Worlds && !p.Worlds) || (selected.Options && !p.Options) || (selected.Servers && !p.Servers) ||
		(selected.Screenshots && !p.Screenshots) {
		return i18n.Error("prism.changed")
	}
	if p.archiveRoot != "" {
		return copyPrismZIP(ctx, p, destination, selected, chosen, commit, report)
	}
	if !filepath.IsAbs(p.Source) {
		return i18n.Error("prism.select_root")
	}
	var root *os.Root
	if selected.Screenshots {
		var info fs.FileInfo
		root, info, err = openPrismScreenshotSource(p.Source)
		if err == nil && (p.directoryScreenshots == nil || !os.SameFile(p.directoryScreenshots.source, info)) {
			root.Close()
			return i18n.Error("prism.changed")
		}
	} else {
		root, err = os.OpenRoot(p.Source)
	}
	if err != nil {
		return prismWorldSourceError(ctx, err, chosen != nil)
	}
	defer root.Close()
	now, err := readPrismProfile(ctx, root.FS(), PrismInstance{Source: p.Source}, filepath.Base(p.Source))
	if err != nil {
		return prismWorldSourceError(ctx, err, chosen != nil)
	}
	expected := p
	expected.Servers, now.Servers = false, false
	expected.serverList, now.serverList = nil, nil
	expected.worldFolders = nil
	expected.Screenshots, expected.directoryScreenshots = false, nil
	if now != expected {
		return prismWorldSourceError(ctx, i18n.Error("prism.changed"), chosen != nil)
	}
	var paths []setupPath
	if chosen != nil {
		for _, name := range slices.Sorted(maps.Keys(chosen)) {
			paths = append(paths, setupPath{"saves/" + name, true})
		}
	} else if selected.Worlds {
		paths = append(paths, setupPath{"saves", true})
	}
	if selected.Options {
		paths = append(paths, setupPath{"options.txt", false})
	}
	var src *os.Root
	if selected.Screenshots {
		var info fs.FileInfo
		src, info, err = openPrismScreenshotDir(root, now.GameFolder)
		if err == nil && !os.SameFile(p.directoryScreenshots.game, info) {
			src.Close()
			return i18n.Error("prism.changed")
		}
	} else {
		src, err = root.OpenRoot(filepath.Join(".", now.GameFolder))
	}
	if err != nil {
		return prismWorldSourceError(ctx, err, chosen != nil)
	}
	defer src.Close()
	if err := p.worldFolders.check(ctx, src, chosen); err != nil {
		return err
	}
	if chosen != nil && commit != nil {
		save := commit
		commit = func() error {
			if err := p.worldFolders.check(ctx, src, chosen); err != nil {
				return err
			}
			return save()
		}
	}
	if selected.Servers && !currentPrismServerFile(src, p.serverList) {
		return i18n.Error("prism.changed")
	}
	return stageNewDirectory(ctx, p.Source, destination, commit, report, func(dst *os.Root) error {
		if err := copySetup(ctx, src, dst, paths, report); err != nil {
			if checkErr := p.worldFolders.check(ctx, src, chosen); checkErr != nil {
				return checkErr
			}
			return err
		}
		if selected.Screenshots {
			if err := p.directoryScreenshots.copy(ctx, root, now.GameFolder, src, dst, report); err != nil {
				return i18n.Error("instance.duplicate_file_failed", "screenshots", err)
			}
		}
		if !selected.Servers {
			return nil
		}
		if report != nil {
			report(tasks.Progress{Stage: i18n.Text("progress.copy_instance_file", serverListPath.name)})
		}
		if err := copySetupFile(ctx, src, dst, serverListPath.name, p.serverList); err != nil {
			return i18n.Error("instance.duplicate_file_failed", serverListPath.name, err)
		}
		if !currentPrismServerFile(src, p.serverList) {
			return i18n.Error("prism.changed")
		}
		return nil
	})
}

func (p PrismInstance) selectedWorldFolders(selected PrismFiles) (map[string]struct{}, error) {
	if selected.WorldFolders == nil {
		return nil, nil
	}
	if !selected.Worlds || !p.Worlds || p.worldFolders == nil || len(selected.WorldFolders) == 0 ||
		len(selected.WorldFolders) > len(p.worldFolders.names) {
		return nil, ErrPrismWorldsChanged
	}
	chosen := make(map[string]struct{}, len(selected.WorldFolders))
	for _, name := range selected.WorldFolders {
		if _, ok := slices.BinarySearch(p.worldFolders.names, name); !ok {
			return nil, ErrPrismWorldsChanged
		}
		chosen[name] = struct{}{}
	}
	if len(chosen) != len(selected.WorldFolders) {
		return nil, ErrPrismWorldsChanged
	}
	return chosen, nil
}

func prismWorldSourceError(ctx context.Context, err error, choosingWorlds bool) error {
	if choosingWorlds && err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return ErrPrismWorldsChanged
	}
	return err
}

// Check only the accepted directory identities. Changes to other entries in saves do not invalidate a selection.
func (p *prismWorldPreview) check(ctx context.Context, source *os.Root, chosen map[string]struct{}) error {
	if chosen == nil {
		return nil
	}
	saves, err := source.Lstat("saves")
	if err != nil || !saves.IsDir() || !os.SameFile(p.saves, saves) {
		return prismWorldSourceError(ctx, ErrPrismWorldsChanged, true)
	}
	for name := range chosen {
		info, err := source.Lstat("saves/" + name)
		want := p.dirs[name]
		if err != nil || !info.IsDir() || !os.SameFile(want, info) || !info.ModTime().Equal(want.ModTime()) {
			return prismWorldSourceError(ctx, ErrPrismWorldsChanged, true)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	return nil
}

func currentPrismServerFile(root *os.Root, expected fs.FileInfo) bool {
	info, err := root.Lstat(serverListPath.name)
	return expected != nil && err == nil && info.Mode().IsRegular() && os.SameFile(info, expected) &&
		info.Size() == expected.Size() && info.ModTime().Equal(expected.ModTime())
}

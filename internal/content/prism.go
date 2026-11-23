package content

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/tasks"
)

// PrismInstance describes only a vanilla profile and the data offered in the preview.
type PrismInstance struct {
	Source, GameFolder, Name, Version string
	Worlds, Options, SkippedSettings  bool
	Launch                            instance.LaunchOptions
	archiveRoot                       string // "." or the single wrapper folder in a ZIP; empty for a directory.
	archiveIndex                      [sha256.Size]byte
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
	return readPrismProfile(ctx, root.FS(), PrismInstance{Source: filepath.Clean(dir)}, filepath.Base(dir))
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
	if (worlds && !p.Worlds) || (options && !p.Options) {
		return i18n.Error("prism.changed")
	}
	if p.archiveRoot != "" {
		return copyPrismZIP(ctx, p, destination, worlds, options, commit, report)
	}
	now, err := ReadPrismInstance(ctx, p.Source)
	if err != nil {
		return err
	}
	if now != p {
		return i18n.Error("prism.changed")
	}
	var paths []setupPath
	if worlds {
		paths = append(paths, setupPath{"saves", true})
	}
	if options {
		paths = append(paths, setupPath{"options.txt", false})
	}
	return copyNewDirectory(ctx, p.Source, filepath.Join(p.Source, p.GameFolder), destination, paths, false, commit, report)
}

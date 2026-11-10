package launcher

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

type ContentKind string

const (
	ResourcePacks  ContentKind = "resourcepacks"
	Mods           ContentKind = "mods"
	ShaderPacks    ContentKind = "shaderpacks"
	maxContentSize             = 512 << 20
)

type ContentFile struct {
	Name    string
	Size    int64
	Enabled bool
}

func (k ContentKind) format(i Instance) (ext string, markers []string, err error) {
	switch k {
	case ResourcePacks:
		return ".zip", []string{"pack.mcmeta"}, nil
	case Mods:
		switch {
		case i.Fabric():
			return ".jar", []string{"fabric.mod.json"}, nil
		case i.Forge():
			return ".jar", []string{"META-INF/mods.toml"}, nil
		case i.NeoForge():
			switch i.Version {
			case "1.20.1", "1.20.2", "1.20.3", "1.20.4":
				return ".jar", []string{"META-INF/mods.toml"}, nil
			default:
				return ".jar", []string{"META-INF/neoforge.mods.toml"}, nil
			}
		case i.Quilt():
			return ".jar", []string{"quilt.mod.json", "fabric.mod.json"}, nil
		}
	case ShaderPacks:
		if i.Fabric() {
			return ".zip", []string{"shaders/"}, nil
		}
	}
	return "", nil, i18n.Error("resource.kind_unsupported")
}

func (k ContentKind) Supports(i Instance) bool {
	_, _, err := k.format(i)
	return err == nil && i.Validate() == nil
}

func contentDirectory(i Instance, kind ContentKind, create bool) (*os.Root, error) {
	if _, _, err := kind.format(i); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(i.GameDir) {
		return nil, i18n.Error("instance.game_dir_required")
	}
	if create {
		if err := os.MkdirAll(i.GameDir, 0755); err != nil {
			return nil, err
		}
	}
	root, err := os.OpenRoot(i.GameDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if create {
		if err := root.Mkdir(string(kind), 0755); err != nil && !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
	}
	return root.OpenRoot(string(kind))
}

func ListContent(i Instance, kind ContentKind) ([]ContentFile, error) {
	ext, _, err := kind.format(i)
	if err != nil {
		return nil, err
	}
	root, err := contentDirectory(i, kind, false)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return nil, err
	}
	var result []ContentFile
	for _, entry := range entries {
		name, disabled := strings.CutSuffix(entry.Name(), ".disabled")
		if !entry.Type().IsRegular() || !strings.EqualFold(filepath.Ext(name), ext) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		result = append(result, ContentFile{Name: name, Size: info.Size(), Enabled: !disabled})
	}
	return result, nil
}

func contentName(name, ext string) bool {
	return filepath.IsLocal(name) && filepath.Base(name) == name &&
		!strings.ContainsAny(name, "/\\:<>|\"?*") && strings.EqualFold(filepath.Ext(name), ext)
}

func SetContentEnabled(i Instance, kind ContentKind, name string, enabled bool) error {
	ext, _, err := kind.format(i)
	if err != nil {
		return err
	}
	if !contentName(name, ext) {
		return i18n.Error("resource.filename_invalid")
	}
	root, err := contentDirectory(i, kind, false)
	if err != nil {
		return err
	}
	defer root.Close()
	from, to := name, name+".disabled"
	if enabled {
		from, to = to, from
	}
	info, err := root.Lstat(from)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return i18n.Error("resource.not_regular")
	}
	if _, err := root.Lstat(to); !errors.Is(err, fs.ErrNotExist) {
		if err != nil {
			return err
		}
		return i18n.Error("resource.name_conflict")
	}
	return root.Rename(from, to)
}

func ImportContent(ctx context.Context, i Instance, kind ContentKind, source string) error {
	f, err := os.Open(source)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return i18n.Error("resource.regular_file_required")
	}
	return writeContent(ctx, i, kind, filepath.Base(source), f, info.Size(), nil, "")
}

func writeContent(ctx context.Context, i Instance, kind ContentKind, name string, in io.Reader, size int64, digest hash.Hash, expected string) (err error) {
	ext, markers, err := kind.format(i)
	if err != nil {
		return err
	}
	if !contentName(name, ext) || size <= 0 || size > maxContentSize {
		return i18n.Error("resource.filename_or_size_invalid")
	}
	root, err := contentDirectory(i, kind, true)
	if err != nil {
		return err
	}
	defer root.Close()
	if _, err := root.Lstat(name + ".disabled"); !errors.Is(err, fs.ErrNotExist) {
		if err != nil {
			return err
		}
		return i18n.Error("resource.disabled_conflict")
	}
	f, err := root.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, f.Close())
		if err != nil {
			_ = root.Remove(name)
		}
	}()
	var out io.Writer = f
	if digest != nil {
		out = io.MultiWriter(f, digest)
	}
	n, err := io.Copy(out, io.LimitReader(transfer.Reader(ctx, in), size+1))
	if err != nil {
		return err
	}
	if n != size || digest != nil && !strings.EqualFold(fmt.Sprintf("%x", digest.Sum(nil)), expected) {
		return i18n.Error("resource.checksum_failed")
	}
	z, err := zip.NewReader(f, n)
	if err != nil {
		return i18n.Error("resource.archive_invalid", err)
	}
	for _, entry := range z.File {
		if !entry.FileInfo().IsDir() {
			for _, marker := range markers {
				if entry.Name == marker || kind == ShaderPacks && strings.HasPrefix(entry.Name, marker) {
					return ctx.Err()
				}
			}
		}
	}
	return i18n.Error("resource.archive_missing_marker", strings.Join(markers, i18n.Text("common.or_separator")))
}

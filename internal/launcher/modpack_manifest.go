package launcher

import (
	"archive/zip"
	"encoding/json"
	"io"
	"path"
	"slices"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

type modpackDownload struct {
	path  string
	asset modpackAsset
}

type modpackOverride struct {
	path string
	file *zip.File
}

type modpackPlan struct {
	info      ModpackInfo
	downloads []modpackDownload
	curse     []curseReference
	overrides []modpackOverride
}

type mrpackManifest struct {
	FormatVersion         int
	Game, Name, VersionID string
	Dependencies          map[string]string
	Files                 []struct {
		Path      string
		FileSize  *int64
		Hashes    struct{ SHA512 string }
		Downloads []string
		Env       struct{ Client string }
	}
}

type curseManifest struct {
	ManifestType, Name, Version, Overrides string
	ManifestVersion                        int
	Minecraft                              struct {
		Version    string
		ModLoaders []struct {
			ID      string
			Primary bool
		}
	}
	Files []curseReference
}

func readModpackManifest(f *zip.File, dst any) error {
	if f == nil {
		return i18n.Error("modpack.manifest_missing")
	}
	if f.UncompressedSize64 > maxMetadata {
		return i18n.Error("modpack.manifest_too_large")
	}
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	data, err := io.ReadAll(io.LimitReader(r, maxMetadata+1))
	if err != nil {
		return err
	}
	if len(data) > maxMetadata {
		return i18n.Error("modpack.manifest_too_large")
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return i18n.Error("modpack.manifest_read_failed", err)
	}
	return nil
}

func readModpack(r io.ReaderAt, size int64, source ModpackSource) (modpackPlan, error) {
	p := modpackPlan{info: ModpackInfo{Source: source}}
	z, err := zip.NewReader(r, size)
	if err != nil {
		return p, i18n.Error("modpack.archive_open_failed", err)
	}
	if len(z.File) > maxModpackEntries {
		return p, i18n.Error("modpack.archive_too_many_files")
	}
	files := make(map[string]*zip.File, len(z.File))
	seen := make(map[string]bool, len(z.File))
	var bytes uint64
	for _, f := range z.File {
		dir := f.Mode().IsDir() && strings.HasSuffix(f.Name, "/")
		name := strings.TrimSuffix(f.Name, "/")
		key := strings.ToLower(name)
		if !storage.ValidPortablePath(name) || seen[key] || (!dir && (!f.Mode().IsRegular() || f.Name != name)) {
			return p, i18n.Error("modpack.archive_path_invalid", f.Name)
		}
		seen[key] = true
		if f.UncompressedSize64 > maxModpackFile || bytes+f.UncompressedSize64 > maxModpackBytes {
			return p, i18n.Error("modpack.archive_expanded_too_large")
		}
		bytes += f.UncompressedSize64
		if !dir {
			files[name] = f
		}
	}
	var layers []string
	var loader, loaderVersion string
	switch source {
	case ModrinthModpacks:
		var m mrpackManifest
		if err := readModpackManifest(files["modrinth.index.json"], &m); err != nil {
			return p, err
		}
		if m.FormatVersion != 1 || m.Game != "minecraft" {
			return p, i18n.Error("modpack.modrinth_manifest_unsupported")
		}
		p.info.Name, p.info.Version, p.info.Minecraft = m.Name, m.VersionID, m.Dependencies["minecraft"]
		for id, v := range m.Dependencies {
			switch id {
			case "minecraft":
			case "fabric-loader", "forge", "neoforge", "quilt-loader":
				if loader != "" {
					return p, i18n.Error("modpack.multiple_loaders")
				}
				loader, loaderVersion = id, v
			default:
				return p, i18n.Error("modpack.required_loader_unsupported", id)
			}
		}
		if len(m.Files) > maxModpackEntries {
			return p, i18n.Error("modpack.too_many_dependencies")
		}
		for _, f := range m.Files {
			switch f.Env.Client {
			case "optional", "unsupported":
				continue
			case "", "required":
			default:
				return p, i18n.Error("modpack.client_requirement_invalid", f.Path)
			}
			if f.FileSize == nil {
				return p, i18n.Error("modpack.dependency_size_missing", f.Path)
			}
			a := modpackAsset{f.Downloads, *f.FileSize, "sha512", f.Hashes.SHA512}
			if !a.valid(maxModpackFile) {
				return p, i18n.Error("modpack.dependency_download_invalid", f.Path)
			}
			p.downloads = append(p.downloads, modpackDownload{f.Path, a})
		}
		layers = []string{"overrides", "client-overrides"}
	case CurseForgeModpacks:
		var m curseManifest
		if err := readModpackManifest(files["manifest.json"], &m); err != nil {
			return p, err
		}
		if m.ManifestType != "minecraftModpack" || m.ManifestVersion != 1 {
			return p, i18n.Error("modpack.curseforge_archive_unsupported")
		}
		p.info.Name, p.info.Version, p.info.Minecraft = m.Name, m.Version, m.Minecraft.Version
		// A manifest can list fallback loaders. Prefer its one designated primary.
		loaders := m.Minecraft.ModLoaders
		n := slices.IndexFunc(loaders, func(l struct {
			ID      string
			Primary bool
		}) bool {
			return l.Primary
		})
		if n >= 0 {
			for _, l := range loaders[n+1:] {
				if l.Primary {
					return p, i18n.Error("modpack.curseforge_multiple_primary_loaders")
				}
			}
		} else if len(loaders) == 1 {
			n = 0
		} else if len(loaders) > 1 {
			return p, i18n.Error("modpack.curseforge_primary_loader_missing")
		}
		if n >= 0 {
			kind, version, ok := strings.Cut(loaders[n].ID, "-")
			if !ok || kind == "" || version == "" {
				return p, i18n.Error("modpack.curseforge_loader_invalid", loaders[n].ID)
			}
			loader, loaderVersion = kind, version
		}
		if len(m.Files) > maxModpackEntries {
			return p, i18n.Error("modpack.too_many_dependencies")
		}
		for _, ref := range m.Files {
			if ref.Required == nil || *ref.Required {
				if ref.ProjectID <= 0 || ref.FileID <= 0 {
					return p, i18n.Error("curseforge.manifest_reference_invalid")
				}
				p.curse = append(p.curse, ref)
			}
		}
		if m.Overrides == "" {
			m.Overrides = "overrides"
		}
		if !storage.ValidPortablePath(m.Overrides) {
			return p, i18n.Error("modpack.curseforge_overrides_invalid")
		}
		layers = []string{m.Overrides}
	default:
		return p, i18n.Error("modpack.source_unknown")
	}
	p.info.Loader, p.info.LoaderVersion, err = instance.NormalizeLoader(p.info.Minecraft, loader, loaderVersion)
	if err != nil {
		return p, err
	}
	for _, layer := range layers {
		for _, f := range z.File {
			name, ok := strings.CutPrefix(f.Name, layer+"/")
			if ok && !f.Mode().IsDir() {
				p.overrides = append(p.overrides, modpackOverride{name, f})
			}
		}
	}
	p.info.Downloads, p.info.Overrides = len(p.downloads)+len(p.curse), len(p.overrides)
	return p, p.validate()
}

func (p modpackPlan) validate() error {
	type entry struct {
		path string
		file bool
	}
	paths := make(map[string]entry)
	var total int64
	add := func(name string, overwrite bool, size int64) error {
		total += size
		if total > maxModpackBytes || !storage.ValidPortablePath(name) {
			return i18n.Error("modpack.plan_size_or_path_invalid", name)
		}
		key := strings.ToLower(name)
		if old, ok := paths[key]; ok && (!overwrite || old != (entry{name, true})) {
			return i18n.Error("modpack.plan_path_conflict", name)
		}
		paths[key] = entry{name, true}
		for dir := path.Dir(name); dir != "."; dir = path.Dir(dir) {
			key := strings.ToLower(dir)
			if old, ok := paths[key]; ok && old != (entry{dir, false}) {
				return i18n.Error("modpack.plan_directory_conflict", dir)
			}
			paths[key] = entry{dir, false}
		}
		return nil
	}
	for _, d := range p.downloads {
		if err := add(d.path, false, d.asset.Size); err != nil {
			return err
		}
	}
	for _, o := range p.overrides {
		if err := add(o.path, true, int64(o.file.UncompressedSize64)); err != nil {
			return err
		}
	}
	return nil
}

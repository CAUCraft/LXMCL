package modpack

import (
	"archive/zip"
	"slices"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

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
	Files []struct {
		ProjectID, FileID int
		Required          *bool
	}
}

func readCurseForge(p *plan, files map[string]*zip.File) ([]string, error) {
	var loader, loaderVersion string
	var m curseManifest
	if err := readManifest(files["manifest.json"], &m); err != nil {
		return nil, err
	}
	if m.ManifestType != "minecraftModpack" || m.ManifestVersion != 1 {
		return nil, i18n.Error("modpack.curseforge_archive_unsupported")
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
				return nil, i18n.Error("modpack.curseforge_multiple_primary_loaders")
			}
		}
	} else if len(loaders) == 1 {
		n = 0
	} else if len(loaders) > 1 {
		return nil, i18n.Error("modpack.curseforge_primary_loader_missing")
	}
	if n >= 0 {
		kind, version, ok := strings.Cut(loaders[n].ID, "-")
		if !ok || kind == "" || version == "" {
			return nil, i18n.Error("modpack.curseforge_loader_invalid", loaders[n].ID)
		}
		loader, loaderVersion = kind, version
	}
	if len(m.Files) > maxEntries {
		return nil, i18n.Error("modpack.too_many_dependencies")
	}
	for _, ref := range m.Files {
		if ref.Required == nil || *ref.Required {
			if ref.ProjectID <= 0 || ref.FileID <= 0 {
				return nil, i18n.Error("curseforge.manifest_reference_invalid")
			}
			p.curse = append(p.curse, Reference{ProjectID: ref.ProjectID, FileID: ref.FileID})
		}
	}
	if m.Overrides == "" {
		m.Overrides = "overrides"
	}
	if !storage.ValidPortablePath(m.Overrides) {
		return nil, i18n.Error("modpack.curseforge_overrides_invalid")
	}
	layers := []string{m.Overrides}

	var err error
	p.info.Loader, p.info.LoaderVersion, err = instance.NormalizeLoader(p.info.Minecraft, loader, loaderVersion)
	return layers, err
}

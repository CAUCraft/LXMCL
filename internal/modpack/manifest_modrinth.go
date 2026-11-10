package modpack

import (
	"archive/zip"

	"github.com/CAUCraft/LXMCL/internal/distribution"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

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

func readModrinth(p *plan, files map[string]*zip.File) ([]string, error) {
	var loader, loaderVersion string
	var m mrpackManifest
	if err := readManifest(files["modrinth.index.json"], &m); err != nil {
		return nil, err
	}
	if m.FormatVersion != 1 || m.Game != "minecraft" {
		return nil, i18n.Error("modpack.modrinth_manifest_unsupported")
	}
	p.info.Name, p.info.Version, p.info.Minecraft = m.Name, m.VersionID, m.Dependencies["minecraft"]
	for id, v := range m.Dependencies {
		switch id {
		case "minecraft":
		case "fabric-loader", "forge", "neoforge", "quilt-loader":
			if loader != "" {
				return nil, i18n.Error("modpack.multiple_loaders")
			}
			loader, loaderVersion = id, v
		default:
			return nil, i18n.Error("modpack.required_loader_unsupported", id)
		}
	}
	if len(m.Files) > maxEntries {
		return nil, i18n.Error("modpack.too_many_dependencies")
	}
	for _, f := range m.Files {
		switch f.Env.Client {
		case "optional", "unsupported":
			continue
		case "", "required":
		default:
			return nil, i18n.Error("modpack.client_requirement_invalid", f.Path)
		}
		if f.FileSize == nil {
			return nil, i18n.Error("modpack.dependency_size_missing", f.Path)
		}
		a := distribution.Asset{URLs: f.Downloads, Size: *f.FileSize, Algorithm: "sha512", Digest: f.Hashes.SHA512}
		if !a.Valid(distribution.MaxFileSize) {
			return nil, i18n.Error("modpack.dependency_download_invalid", f.Path)
		}
		p.downloads = append(p.downloads, Download{f.Path, a})
	}
	layers := []string{"overrides", "client-overrides"}

	var err error
	p.info.Loader, p.info.LoaderVersion, err = instance.NormalizeLoader(p.info.Minecraft, loader, loaderVersion)
	return layers, err
}

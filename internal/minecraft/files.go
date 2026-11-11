package minecraft

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

func (g Runtime) binaries(m metadata) ([]fileDownload, []string, error) {
	client := filepath.Join(g.versionDir(m.ID), "client.jar")
	files := []fileDownload{{client, m.Downloads.Client}}
	seen := make(map[string]artifact)
	var cp []string
	for _, lib := range m.Libraries {
		if !allowed(lib.Rules, nil) {
			continue
		}
		if len(lib.Natives) != 0 {
			return nil, nil, i18n.Error("metadata.legacy_natives_unsupported")
		}
		file := lib.Downloads.Artifact
		if !filepath.IsLocal(file.Path) || filepath.Clean(file.Path) == "." {
			return nil, nil, i18n.Error("metadata.library_path_invalid", file.Path)
		}
		path := filepath.Join(g.Cache, "libraries", filepath.FromSlash(file.Path))
		key := path
		if runtime.GOOS == "windows" {
			key = strings.ToLower(path)
		}
		if prev, ok := seen[key]; ok {
			if !strings.EqualFold(prev.SHA, file.SHA) || prev.Size != file.Size {
				return nil, nil, i18n.Error("metadata.library_conflict", file.Path)
			}
			continue
		}
		seen[key] = file
		files = append(files, fileDownload{path, file})
		cp = append(cp, path)
	}
	if m.Logging.Client.Argument != "" {
		f := m.Logging.Client.File
		if !filepath.IsLocal(f.ID) || filepath.Base(f.ID) != f.ID {
			return nil, nil, i18n.Error("metadata.log_config_invalid")
		}
		files = append(files, fileDownload{filepath.Join(g.Cache, "assets", "log_configs", f.ID), f.artifact})
	}
	return files, append(cp, client), nil
}

func (g Runtime) indexPath(m metadata) (string, error) {
	if !versionID.MatchString(m.AssetIndex.ID) {
		return "", i18n.Error("metadata.asset_index_invalid")
	}
	return filepath.Join(g.Cache, "assets", "indexes", m.AssetIndex.ID+".json"), nil
}

func (g Runtime) assets(indexPath string) ([]fileDownload, error) {
	var assets assetIndex
	if found, err := storage.ReadJSON(indexPath, &assets); err != nil || !found {
		return nil, errors.Join(i18n.Error("metadata.asset_index_read_failed"), err)
	}
	files := make([]fileDownload, 0, len(assets.Objects))
	seen := make(map[string]int64)
	for _, a := range assets.Objects {
		hash := strings.ToLower(a.Hash)
		obj := artifact{SHA: hash, Size: a.Size}
		if _, err := obj.Checksum(); err != nil || a.Size < 0 {
			return nil, i18n.Error("metadata.asset_invalid")
		}
		if size, ok := seen[hash]; ok {
			if size != a.Size {
				return nil, i18n.Error("metadata.asset_size_conflict")
			}
			continue
		}
		seen[hash] = a.Size
		key := hash[:2] + "/" + hash
		obj.URL = "https://resources.download.minecraft.net/" + key
		files = append(files, fileDownload{filepath.Join(g.Cache, "assets", "objects", filepath.FromSlash(key)), obj})
	}
	return files, nil
}

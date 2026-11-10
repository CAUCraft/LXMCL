package launcher

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

type rule struct {
	Action   string
	Features map[string]bool
	OS       struct{ Name, Arch, Version string }
}

type metadata struct {
	ID, Type, MainClass string
	Arguments           struct{ JVM, Game []json.RawMessage }
	JavaVersion         struct {
		MajorVersion int
		Component    string
	}
	Downloads  struct{ Client artifact }
	AssetIndex struct {
		ID string
		artifact
	}
	Libraries []struct {
		Name      string
		Rules     []rule
		Downloads struct{ Artifact artifact }
		Natives   map[string]string
	}
	Logging struct {
		Client struct {
			Argument string
			File     struct {
				ID string
				artifact
			}
		}
	}
}

type assetIndex struct {
	Objects map[string]struct {
		Hash string
		Size int64
	}
}

func (g Game) versionDir(id string) string { return filepath.Join(g.Cache, "versions", id) }

func (g Game) readVersion(id, expectedSHA string) (metadata, error) {
	var m metadata
	f, err := os.Open(filepath.Join(g.versionDir(id), "version.json"))
	if err != nil {
		return m, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxMetadata+1))
	if err != nil {
		return m, err
	}
	sum := sha1.Sum(b)
	if len(b) > maxMetadata || (expectedSHA != "" && !strings.EqualFold(hex.EncodeToString(sum[:]), expectedSHA)) {
		return m, i18n.Error("metadata.checksum_failed")
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return m, i18n.Error("metadata.read_failed", err)
	}
	if m.ID != id || m.Type != "release" || m.MainClass == "" || len(m.Arguments.JVM) == 0 || m.JavaVersion.MajorVersion < 17 {
		return m, i18n.Error("metadata.version_invalid")
	}
	return m, nil
}

func (g Game) binaries(m metadata) ([]fileDownload, []string, error) {
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

func (g Game) indexPath(m metadata) (string, error) {
	if !versionID.MatchString(m.AssetIndex.ID) {
		return "", i18n.Error("metadata.asset_index_invalid")
	}
	return filepath.Join(g.Cache, "assets", "indexes", m.AssetIndex.ID+".json"), nil
}

func (g Game) assets(indexPath string) ([]fileDownload, error) {
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

func (g Game) downloadVersion(ctx context.Context, id string, d *downloads) (metadata, Version, error) {
	all, err := d.releases(ctx)
	if err != nil {
		return metadata{}, Version{}, err
	}
	index := slices.IndexFunc(all, func(v Version) bool { return v.ID == id })
	if index == -1 {
		return metadata{}, Version{}, i18n.Error("version.release_not_found", id)
	}
	v := all[index]
	if err := d.file(ctx, filepath.Join(g.versionDir(id), "version.json"), artifact{URL: v.URL, SHA: v.SHA, Size: unknownSize}); err != nil {
		return metadata{}, Version{}, err
	}
	m, err := g.readVersion(id, v.SHA)
	return m, v, err
}

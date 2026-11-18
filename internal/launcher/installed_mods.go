package launcher

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"

	"github.com/CAUCraft/LXMCL/internal/catalog"
	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

type InstalledMod struct {
	File   ContentFile
	Digest string
	catalog.ModMatch
	Unconfirmed []ModDependency
}

func (m InstalledMod) DiskName() string {
	if !m.File.Enabled {
		return m.File.Name + ".disabled"
	}
	return m.File.Name
}

func modDigest(ctx context.Context, root *os.Root, name string) (string, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() > content.MaxSize {
		return "", i18n.Error("resource.filename_or_size_invalid")
	}
	f, err := root.Open(name)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hash := sha512.New()
	n, err := io.Copy(hash, io.LimitReader(transfer.Reader(ctx, f), content.MaxSize+1))
	if err != nil {
		return "", err
	}
	if n != info.Size() {
		return "", i18n.Error("resource.filename_or_size_invalid")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (g Game) CheckMods(ctx context.Context, i Instance) (map[string]InstalledMod, error) {
	files, err := ListContent(i, Mods)
	if err != nil || len(files) == 0 {
		return nil, err
	}
	root, err := content.OpenDirectory(i, Mods, false)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	result := make(map[string]InstalledMod, len(files))
	var hashes []string
	for _, file := range files {
		m := InstalledMod{File: file}
		if file.Size > content.MaxSize {
			continue
		}
		m.Digest, err = modDigest(ctx, root, m.DiskName())
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file.Name, err)
		}
		result[m.DiskName()] = m
		hashes = append(hashes, m.Digest)
	}
	slices.Sort(hashes)
	matches, err := (catalog.Client{Transport: g.Transport}).MatchInstalledMods(ctx, i, slices.Compact(hashes))
	if err != nil {
		return nil, err
	}
	for name, m := range result {
		m.ModMatch = matches[m.Digest]
		result[name] = m
	}
	projects, versions := map[string]bool{}, map[string]bool{}
	for _, m := range result {
		if m.File.Enabled && m.Current.ProjectID != "" {
			projects[m.Current.ProjectID] = true
			versions[m.Current.ID] = true
		}
	}
	for k, m := range result {
		v := m.Current
		if m.Update.ID != "" {
			v = m.Update
		}
		for _, dep := range v.RequiredDependencies() {
			if dep.VersionID != "" && !versions[dep.VersionID] || dep.VersionID == "" && !projects[dep.ProjectID] {
				m.Unconfirmed = append(m.Unconfirmed, dep)
			}
		}
		result[k] = m
	}
	return result, nil
}

// UpdateMod preserves the installed filename, including its disabled suffix.
func (g Game) UpdateMod(ctx context.Context, i Instance, m InstalledMod) error {
	if !content.ValidName(m.File.Name, ".jar") || m.Digest == "" || m.Current.ProjectID == "" || m.Update.ProjectID != m.Current.ProjectID || !m.Update.Compatible(i, Mods) {
		return i18n.Error("resource.filename_or_size_invalid")
	}
	_, asset, err := m.Update.DownloadAsset(Mods)
	if err != nil {
		return err
	}
	root, err := content.OpenDirectory(i, Mods, false)
	if err != nil {
		return err
	}
	defer root.Close()
	return asset.Fetch(ctx, &http.Client{Transport: g.Transport}, func(ctx context.Context, r io.Reader) error {
		return storage.ReplaceFile(ctx, root, m.DiskName(), 0644, func(f *os.File) error {
			if err := asset.CopyVerified(ctx, f, r); err != nil {
				return err
			}
			if err := content.ValidateArchive(ctx, i, Mods, f, asset.Size); err != nil {
				return err
			}
			hash, err := modDigest(ctx, root, m.DiskName())
			if err != nil {
				return err
			}
			if hash != m.Digest {
				return i18n.Error("resource.file_name_conflict", m.DiskName())
			}
			return nil
		})
	})
}

package catalog

import (
	"crypto/sha512"
	"encoding/hex"
	"net/url"
	"slices"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/distribution"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

func (v PackVersion) Compatible(i instance.Instance, kind content.Kind) bool {
	if !slices.Contains(v.GameVersions, i.Version) {
		return false
	}
	return kind != content.Mods || slices.Contains(v.Loaders, modrinthModLoader(i)) ||
		i.Quilt() && v.ProjectID == fabricAPIProject && slices.Contains(v.Loaders, "fabric")
}

func (v PackVersion) DownloadFile(kind content.Kind) (PackFile, bool) {
	if modrinthType(kind) == "" {
		return PackFile{}, false
	}
	ext := ".zip"
	if kind == content.Mods {
		ext = ".jar"
	}
	valid := func(f PackFile) bool { return content.ValidName(f.Filename, ext) }
	n := slices.IndexFunc(v.Files, func(f PackFile) bool { return f.Primary && valid(f) })
	if n == -1 {
		n = slices.IndexFunc(v.Files, valid)
	}
	if n == -1 {
		return PackFile{}, false
	}
	return v.Files[n], true
}

// DownloadAsset validates the selected file without changing primary-file selection.
// DownloadFile remains available for presentation of incomplete provider metadata.
func (v PackVersion) DownloadAsset(kind content.Kind) (PackFile, distribution.Asset, error) {
	file, ok := v.DownloadFile(kind)
	if !ok {
		return PackFile{}, distribution.Asset{}, i18n.Error("modrinth.version_file_missing")
	}
	u, err := url.Parse(file.URL)
	if err != nil || u.Hostname() != "cdn.modrinth.com" || !distribution.AllowedURL(u) {
		return PackFile{}, distribution.Asset{}, i18n.Error("modrinth.download_source_invalid")
	}
	if digest, err := hex.DecodeString(file.Hashes.SHA512); err != nil || len(digest) != sha512.Size {
		return PackFile{}, distribution.Asset{}, i18n.Error("modrinth.sha512_invalid")
	}
	if file.Size <= 0 || file.Size > content.MaxSize {
		return PackFile{}, distribution.Asset{}, i18n.Error("modrinth.file_size_invalid")
	}
	return file, distribution.Asset{URLs: []string{file.URL}, Size: file.Size, Algorithm: "sha512", Digest: file.Hashes.SHA512}, nil
}

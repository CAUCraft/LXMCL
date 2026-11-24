package minecraft

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

// ReleaseJavaMajor reads the selected release's declared Java major version.
// A release without a URL is a local-only lookup. Zero means the declaration is unknown.
func (g Runtime) ReleaseJavaMajor(ctx context.Context, release Version) (int, error) {
	id := release.ID
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if !instance.SupportedRelease(id) {
		return 0, i18n.Error("version.select_supported")
	}
	if filepath.IsAbs(g.Cache) && regularVersionFile(g.versionDir(id), "verified") && regularVersionFile(g.versionDir(id), "version.json") {
		receipt, err := g.readReceipt(id)
		if err == nil && receipt.VersionSHA != "" &&
			(release.SHA == "" || strings.EqualFold(receipt.VersionSHA, release.SHA)) {
			raw, err := g.readVersionBytes(id, receipt.VersionSHA)
			if err == nil {
				major, err := declaredJavaMajor(raw, id)
				if err == nil {
					if err := ctx.Err(); err != nil {
						return 0, err
					}
					return major, nil
				}
			}
		}
	}
	if err := ctx.Err(); err != nil || release.URL == "" {
		return 0, err
	}

	metaURL, err := url.Parse(release.URL)
	if err != nil || !mojangMetadataURL(metaURL) || release.Type != "release" {
		return 0, i18n.Error("version.catalog_release_invalid")
	}
	if _, err := (artifact{SHA: release.SHA, Size: unknownSize}).Checksum(); err != nil {
		return 0, err
	}
	var major int
	err = downloadMetadata(ctx, g.Client.mojangMetadataDownloads(""), release.URL, func(raw []byte) error {
		if err := checkVersionSHA(raw, release.SHA); err != nil {
			return err
		}
		var err error
		major, err = declaredJavaMajor(raw, id)
		return err
	})
	if err != nil {
		return 0, err
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return major, nil
}

func regularVersionFile(dir, name string) bool {
	info, err := os.Lstat(filepath.Join(dir, name))
	return err == nil && info.Mode().IsRegular()
}

func declaredJavaMajor(raw []byte, id string) (int, error) {
	var declared struct {
		ID, Type    string
		JavaVersion struct{ MajorVersion int }
	}
	if err := json.Unmarshal(raw, &declared); err != nil {
		return 0, i18n.Error("metadata.read_failed", err)
	}
	if declared.ID != id || declared.Type != "release" {
		return 0, i18n.Error("metadata.version_invalid")
	}
	return max(0, declared.JavaVersion.MajorVersion), nil
}

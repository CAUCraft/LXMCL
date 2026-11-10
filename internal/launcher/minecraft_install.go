package launcher

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

func (g Game) Installed(i Instance) bool {
	if i.Validate() != nil {
		return false
	}
	if _, err := g.readReceipt(i.Version); err != nil {
		return false
	}
	for _, name := range []string{"version.json", "client.jar"} {
		st, err := os.Stat(filepath.Join(g.versionDir(i.Version), name))
		if err != nil || !st.Mode().IsRegular() || st.Size() == 0 {
			return false
		}
	}
	if i.UsesKnot() {
		_, err := g.readKnot(i)
		return err == nil
	}
	if i.UsesInstaller() {
		_, err := g.readForge(i)
		return err == nil
	}
	return true
}

type installReceipt struct {
	Platform   string `json:"platform"`
	VersionSHA string `json:"version_sha1"`
}

func platformKey() string { return runtime.GOOS + "/" + runtime.GOARCH }

func (g Game) readReceipt(id string) (installReceipt, error) {
	var r installReceipt
	f, err := os.Open(filepath.Join(g.versionDir(id), "verified"))
	if err != nil {
		return r, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 1025))
	if err != nil {
		return r, err
	}
	if len(b) > 1024 {
		return r, i18n.Error("install.receipt_corrupt")
	}
	// Existing installations did not persist the catalog SHA-1. Continue checking
	// their complete artifacts, and upgrade the receipt after an explicit repair.
	if strings.TrimSpace(string(b)) == "1" {
		return r, nil
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return r, i18n.Error("install.receipt_corrupt")
	}
	if r.Platform != platformKey() {
		return r, i18n.Error("install.platform_mismatch")
	}
	if _, err := (artifact{SHA: r.VersionSHA, Size: unknownSize}).Checksum(); err != nil {
		return r, i18n.Error("install.receipt_checksum_missing")
	}
	return r, nil
}

func (g Game) Install(ctx context.Context, i Instance, report func(Progress)) error {
	return g.InstallWithJava(ctx, i, "", report)
}

func (g Game) InstallWithJava(ctx context.Context, i Instance, javaPath string, report func(Progress)) error {
	return g.installWithJava(ctx, i, javaPath, report, &downloads{http: officialHTTP})
}

func (g Game) install(ctx context.Context, i Instance, report func(Progress), d *downloads) error {
	return g.installWithJava(ctx, i, "", report, d)
}

func (g Game) installWithJava(ctx context.Context, i Instance, javaPath string, report func(Progress), d *downloads) error {
	if err := i.Validate(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(i.GameDir, 0755); err != nil {
		return i18n.Error("install.instance_dir_create_failed", err)
	}
	if report == nil {
		report = func(Progress) {}
	}
	report(Progress{Stage: i18n.Text("progress.fetch_version_metadata")})
	m, v, err := g.downloadVersion(ctx, i.Version, d)
	if err != nil {
		return err
	}
	dir := g.versionDir(i.Version)
	files, _, err := g.binaries(m)
	if err != nil {
		return err
	}
	if err := d.batch(ctx, i18n.Text("progress.verify_game_libraries"), files, report); err != nil {
		return err
	}
	indexPath, err := g.indexPath(m)
	if err != nil {
		return err
	}
	if err := d.file(ctx, indexPath, m.AssetIndex.artifact); err != nil {
		return err
	}
	assets, err := g.assets(indexPath)
	if err != nil {
		return err
	}
	if err := d.batch(ctx, i18n.Text("progress.verify_game_assets"), assets, report); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	receipt, err := json.Marshal(installReceipt{Platform: platformKey(), VersionSHA: v.SHA})
	if err != nil {
		return err
	}
	if err := storage.WriteFile(filepath.Join(dir, "verified"), receipt); err != nil {
		return err
	}
	if i.UsesKnot() {
		return g.installKnot(ctx, i, d, report)
	}
	if i.UsesInstaller() {
		return g.installForge(ctx, i, m, javaPath, d, report)
	}
	return nil
}

func verifyArtifact(ctx context.Context, file fileDownload) error {
	digest, err := file.file.Checksum()
	if err != nil {
		return i18n.Error("common.context_error", filepath.Base(file.path), err)
	}
	ok, err := transfer.Check(ctx, file.path, file.file, digest, maxMetadata)
	if err != nil {
		return err
	}
	if !ok {
		return i18n.Error("install.file_unverified", filepath.Base(file.path))
	}
	return nil
}

func (g Game) verifyInstalled(ctx context.Context, i Instance) (metadata, []string, error) {
	r, err := g.readReceipt(i.Version)
	if errors.Is(err, os.ErrNotExist) {
		err = i18n.Error("install.receipt_missing")
	}
	if err != nil {
		return metadata{}, nil, err
	}
	m, err := g.readVersion(i.Version, r.VersionSHA)
	if err != nil {
		return m, nil, err
	}
	files, cp, err := g.binaries(m)
	if err != nil {
		return m, nil, err
	}
	index, err := g.indexPath(m)
	if err != nil {
		return m, nil, err
	}
	// Check the asset index before trusting its references to other files.
	if err := verifyArtifact(ctx, fileDownload{index, m.AssetIndex.artifact}); err != nil {
		return m, nil, err
	}
	assets, err := g.assets(index)
	if err != nil {
		return m, nil, err
	}
	return m, cp, transfer.Batch(ctx, append(files, assets...), verifyArtifact, nil)
}

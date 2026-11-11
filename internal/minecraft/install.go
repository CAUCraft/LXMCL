package minecraft

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

// Installed is a cheap presence check. Start verifies all referenced files before
// launching; callers may explicitly Install again to repair damaged cache entries.
func (g Runtime) Installed(i instance.Instance) bool {
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

func (g Runtime) Install(ctx context.Context, i instance.Instance, report func(Progress)) error {
	return g.InstallWithJava(ctx, i, "", report)
}

func (g Runtime) InstallWithJava(ctx context.Context, i instance.Instance, javaPath string, report func(Progress)) error {
	return g.installWithJava(ctx, i, javaPath, report, g.Client.downloads())
}

func (g Runtime) install(ctx context.Context, i instance.Instance, report func(Progress), d *downloads) error {
	return g.installWithJava(ctx, i, "", report, d)
}

func (g Runtime) installWithJava(ctx context.Context, i instance.Instance, javaPath string, report func(Progress), d *downloads) error {
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

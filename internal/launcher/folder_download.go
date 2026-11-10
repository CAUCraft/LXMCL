package launcher

import (
	"context"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/distribution"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/modpack"
)

func (g Game) DownloadResourceFile(ctx context.Context, kind ContentKind, version PackVersion, folder string, report func(Progress)) error {
	f, ok := version.DownloadFile(kind)
	u, err := url.Parse(f.URL)
	if !ok || modrinthType(kind) == "" || err != nil || u.Hostname() != "cdn.modrinth.com" || !officialURL(u) {
		return i18n.Error("resource.no_download")
	}
	a := distribution.Asset{URLs: []string{f.URL}, Size: f.Size, Algorithm: "sha512", Digest: f.Hashes.SHA512}
	if a.Size <= 0 || !a.Valid(content.MaxSize) {
		return i18n.Error("resource.size_or_sha512_invalid")
	}
	return g.saveDownload(ctx, a, f.Filename, folder, report)
}

func (r ModpackRelease) Downloadable() bool {
	ext := ".mrpack"
	if r.source == CurseForgeModpacks {
		ext = ".zip"
	}
	return (r.source == ModrinthModpacks || r.source == CurseForgeModpacks) && !r.downloadBlocked &&
		content.ValidName(r.Filename, ext) && r.asset.Size > 0 && r.asset.Valid(modpack.MaxArchiveSize)
}

func (g Game) DownloadModpackFile(ctx context.Context, release ModpackRelease, folder string, report func(Progress)) error {
	if !release.Downloadable() {
		return i18n.Error("modpack.folder_file_unavailable")
	}
	return g.saveDownload(ctx, release.asset, release.Filename, folder, report)
}

func (g Game) saveDownload(ctx context.Context, a distribution.Asset, name, folder string, report func(Progress)) error {
	if !filepath.IsAbs(folder) {
		return i18n.Error("download.folder_required")
	}
	root, err := os.OpenRoot(folder)
	if err != nil {
		return i18n.Error("download.folder_open_failed", err)
	}
	defer root.Close()
	if _, err := root.Lstat(name); err == nil {
		return i18n.Error("download.folder_name_conflict", name)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if report != nil {
		report(Progress{Stage: i18n.Text("progress.download_and_verify", name), Total: 1})
	}
	verified, err := (distribution.Cache{Root: g.Cache}).Ensure(ctx, a)
	if err != nil {
		return err
	}
	src, err := os.Open(verified)
	if err != nil {
		return err
	}
	defer src.Close()
	if err := writeFolderDownload(ctx, root, name, a, src); err != nil {
		return err
	}
	if report != nil {
		report(Progress{Stage: i18n.Text("progress.saved_file", name), Done: 1, Total: 1})
	}
	return nil
}

func writeFolderDownload(ctx context.Context, root *os.Root, name string, a distribution.Asset, src io.Reader) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	err = a.CopyVerified(ctx, f, src)
	if err == nil {
		err = ctx.Err()
	}
	if err = errors.Join(err, f.Close()); err != nil {
		_ = root.Remove(name)
	}
	return err
}

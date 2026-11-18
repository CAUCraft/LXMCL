package launcher

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/distribution"
	"github.com/CAUCraft/LXMCL/internal/i18n"
)

func (g Game) DownloadResourceFile(ctx context.Context, kind ContentKind, version PackVersion, folder string, report func(Progress)) error {
	f, a, err := version.DownloadAsset(kind)
	if err != nil {
		return err
	}
	return g.saveDownload(ctx, a, f.Filename, folder, report)
}

func (g Game) DownloadModpackFile(ctx context.Context, release ModpackRelease, folder string, report func(Progress)) error {
	a, ok := release.DownloadAsset()
	if !ok {
		return i18n.Error("modpack.folder_file_unavailable")
	}
	return g.saveDownload(ctx, a, release.Filename, folder, report)
}

func (g Game) saveDownload(ctx context.Context, a distribution.Asset, name, folder string, report func(Progress)) error {
	return g.downloads().Export(ctx, a, name, folder, func(done, total int) {
		if report == nil {
			return
		}
		key := "progress.download_and_verify"
		if done == total {
			key = "progress.saved_file"
		}
		report(Progress{Stage: i18n.Text(key, name), Done: done, Total: total})
	})
}

// ResumeResourceFile handles a shutdown after file publication and before the queue was saved.
func (g Game) ResumeResourceFile(ctx context.Context, kind ContentKind, v PackVersion, folder string, report func(Progress)) error {
	f, asset, err := v.DownloadAsset(kind)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(folder) {
		return i18n.Error("download.folder_required")
	}
	root, err := os.OpenRoot(folder)
	if err != nil {
		return err
	}
	defer root.Close()
	st, err := root.Lstat(f.Filename)
	if errors.Is(err, os.ErrNotExist) {
		return g.saveDownload(ctx, asset, f.Filename, folder, report)
	}
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() || st.Size() != asset.Size {
		return i18n.Error("download.folder_name_conflict", f.Filename)
	}
	file, err := root.Open(f.Filename)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := asset.CopyVerified(ctx, io.Discard, file); err != nil {
		return err
	}
	if report != nil {
		report(Progress{Stage: i18n.Text("progress.saved_file", f.Filename), Done: 1, Total: 1})
	}
	return nil
}

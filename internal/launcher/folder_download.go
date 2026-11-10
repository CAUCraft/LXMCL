package launcher

import (
	"context"

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
	return (distribution.Cache{Root: g.Cache}).Export(ctx, a, name, folder, func(done, total int) {
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

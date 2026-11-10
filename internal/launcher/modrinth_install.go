package launcher

import (
	"context"
	"net/http"

	"github.com/CAUCraft/LXMCL/internal/catalog"
	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/i18n"
)

func DownloadModrinth(ctx context.Context, i Instance, kind ContentKind, version PackVersion, report func(Progress)) error {
	return downloadModrinth(ctx, i, kind, version, nil, report)
}

func downloadModrinth(ctx context.Context, i Instance, kind ContentKind, version PackVersion, client *http.Client, report func(Progress)) error {
	if _, _, err := kind.Format(i); err != nil {
		return err
	}
	if kind == Mods && (!catalog.ValidModrinthID(version.ProjectID) || !catalog.ValidModrinthID(version.ID)) {
		return i18n.Error("modrinth.mod_project_or_version_invalid")
	}
	if report == nil {
		report = func(Progress) {}
	}
	stage := i18n.Text("progress.download_resources")
	report(Progress{Stage: stage, Total: 1})
	if !version.Compatible(i, kind) {
		return i18n.Error("modrinth.resource_incompatible")
	}
	file, asset, err := version.DownloadAsset(kind)
	if err != nil {
		return err
	}
	if err := content.Install(ctx, i, kind, file.Filename, asset, client); err != nil {
		return err
	}
	report(Progress{Stage: stage, Done: 1, Total: 1})
	return nil
}

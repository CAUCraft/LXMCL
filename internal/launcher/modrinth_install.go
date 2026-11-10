package launcher

import (
	"context"
	"crypto/sha512"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/catalog"
	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

func DownloadModrinth(ctx context.Context, i Instance, kind ContentKind, version PackVersion, report func(Progress)) error {
	return downloadModrinth(ctx, i, kind, version, &downloads{http: officialHTTP}, report)
}

func downloadModrinth(ctx context.Context, i Instance, kind ContentKind, version PackVersion, d *downloads, report func(Progress)) error {
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
	if err := downloadModrinthFile(ctx, i, kind, version, d); err != nil {
		return err
	}
	report(Progress{Stage: stage, Done: 1, Total: 1})
	return nil
}

func downloadModrinthFile(ctx context.Context, i Instance, kind ContentKind, v PackVersion, d *downloads) error {
	if !v.Compatible(i, kind) {
		return i18n.Error("modrinth.resource_incompatible")
	}
	file, _, err := v.DownloadAsset(kind)
	if err != nil {
		return err
	}
	existing, err := alreadyDownloaded(ctx, i, kind, file)
	if err != nil || existing {
		return err
	}
	return retry(ctx, func() error {
		res, err := d.get(ctx, file.URL)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		return content.WriteVerified(ctx, i, kind, file.Filename, res.Body, file.Size, sha512.New(), file.Hashes.SHA512)
	})
}

func alreadyDownloaded(ctx context.Context, i Instance, kind ContentKind, file PackFile) (bool, error) {
	root, err := content.OpenDirectory(i, kind, false)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer root.Close()
	info, err := root.Lstat(file.Filename)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() || info.Size() != file.Size {
		return false, i18n.Error("modrinth.file_name_conflict", file.Filename)
	}
	src, err := root.Open(file.Filename)
	if err != nil {
		return false, err
	}
	defer src.Close()
	h := sha512.New()
	n, err := io.Copy(h, io.LimitReader(transfer.Reader(ctx, src), file.Size+1))
	if err != nil {
		return false, err
	}
	if n != file.Size || !strings.EqualFold(fmt.Sprintf("%x", h.Sum(nil)), file.Hashes.SHA512) {
		return false, i18n.Error("modrinth.file_content_conflict", file.Filename)
	}
	if err := content.ValidateArchive(ctx, i, kind, src, n); err != nil {
		return false, err
	}
	return true, nil
}

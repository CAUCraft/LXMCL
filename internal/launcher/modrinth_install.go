package launcher

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"slices"
	"strings"

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
	if kind == Mods && (!modrinthID.MatchString(version.ProjectID) || !modrinthID.MatchString(version.ID)) {
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

func (v PackVersion) compatible(i Instance, kind ContentKind) bool {
	if !slices.Contains(v.GameVersions, i.Version) {
		return false
	}
	return kind != Mods || slices.Contains(v.Loaders, modrinthModLoader(i)) ||
		i.Quilt() && v.ProjectID == fabricAPIProject && slices.Contains(v.Loaders, "fabric")
}

func (v PackVersion) DownloadFile(kind ContentKind) (PackFile, bool) {
	ext := ".zip"
	if kind == Mods {
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

func downloadModrinthFile(ctx context.Context, i Instance, kind ContentKind, v PackVersion, d *downloads) error {
	if !v.compatible(i, kind) {
		return i18n.Error("modrinth.resource_incompatible")
	}
	file, ok := v.DownloadFile(kind)
	if !ok {
		return i18n.Error("modrinth.version_file_missing")
	}
	u, err := url.Parse(file.URL)
	if err != nil || u.Hostname() != "cdn.modrinth.com" || !officialURL(u) {
		return i18n.Error("modrinth.download_source_invalid")
	}
	if digest, err := hex.DecodeString(file.Hashes.SHA512); err != nil || len(digest) != sha512.Size {
		return i18n.Error("modrinth.sha512_invalid")
	}
	if file.Size <= 0 || file.Size > content.MaxSize {
		return i18n.Error("modrinth.file_size_invalid")
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

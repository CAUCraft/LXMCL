package launcher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
)

func (g Game) DownloadResourceFile(ctx context.Context, kind ContentKind, version PackVersion, folder string, report func(Progress)) error {
	f, ok := version.DownloadFile(kind)
	u, err := url.Parse(f.URL)
	if !ok || kind.modrinthType() == "" || err != nil || u.Hostname() != "cdn.modrinth.com" || !officialURL(u) {
		return errors.New("此资源没有可下载的文件")
	}
	a := modpackAsset{URLs: []string{f.URL}, Size: f.Size, Algorithm: "sha512", Digest: f.Hashes.SHA512}
	if a.Size <= 0 || !a.valid(maxContentSize) {
		return errors.New("资源文件缺少有效的大小或 SHA-512")
	}
	return g.saveDownload(ctx, a, f.Filename, folder, report)
}

func (r ModpackRelease) Downloadable() bool {
	ext := ".mrpack"
	if r.source == CurseForgeModpacks {
		ext = ".zip"
	}
	return (r.source == ModrinthModpacks || r.source == CurseForgeModpacks) && !r.downloadBlocked &&
		contentName(r.Filename, ext) && r.asset.Size > 0 && r.asset.valid(maxModpackArchive)
}

func (g Game) DownloadModpackFile(ctx context.Context, release ModpackRelease, folder string, report func(Progress)) error {
	if !release.Downloadable() {
		return errors.New("此整合包版本没有可校验的下载文件")
	}
	return g.saveDownload(ctx, release.asset, release.Filename, folder, report)
}

func (g Game) saveDownload(ctx context.Context, a modpackAsset, name, folder string, report func(Progress)) error {
	if !filepath.IsAbs(folder) {
		return errors.New("请选择下载文件夹")
	}
	root, err := os.OpenRoot(folder)
	if err != nil {
		return fmt.Errorf("无法打开下载文件夹：%w", err)
	}
	defer root.Close()
	if _, err := root.Lstat(name); err == nil {
		return fmt.Errorf("文件夹已有同名文件：%s", name)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if report != nil {
		report(Progress{Stage: "下载并校验 " + name, Total: 1})
	}
	verified, err := g.modpackAsset(ctx, a, modpackHTTP)
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
		report(Progress{Stage: "已保存 " + name, Done: 1, Total: 1})
	}
	return nil
}

func writeFolderDownload(ctx context.Context, root *os.Root, name string, a modpackAsset, src io.Reader) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	err = a.copyVerified(ctx, f, src)
	if err == nil {
		err = ctx.Err()
	}
	if err = errors.Join(err, f.Close()); err != nil {
		_ = root.Remove(name)
	}
	return err
}

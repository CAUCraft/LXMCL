package content

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

func Import(ctx context.Context, i instance.Instance, kind Kind, source string) error {
	f, err := os.Open(source)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return i18n.Error("resource.regular_file_required")
	}
	return WriteVerified(ctx, i, kind, filepath.Base(source), f, info.Size(), nil, "")
}

func WriteVerified(ctx context.Context, i instance.Instance, kind Kind, name string, in io.Reader, size int64, digest hash.Hash, expected string) (err error) {
	ext, markers, err := kind.Format(i)
	if err != nil {
		return err
	}
	if !ValidName(name, ext) || size <= 0 || size > MaxSize {
		return i18n.Error("resource.filename_or_size_invalid")
	}
	root, err := OpenDirectory(i, kind, true)
	if err != nil {
		return err
	}
	defer root.Close()
	if _, err := root.Lstat(name + ".disabled"); !errors.Is(err, fs.ErrNotExist) {
		if err != nil {
			return err
		}
		return i18n.Error("resource.disabled_conflict")
	}
	f, err := root.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, f.Close())
		if err != nil {
			_ = root.Remove(name)
		}
	}()
	var out io.Writer = f
	if digest != nil {
		out = io.MultiWriter(f, digest)
	}
	n, err := io.Copy(out, io.LimitReader(transfer.Reader(ctx, in), size+1))
	if err != nil {
		return err
	}
	if n != size || digest != nil && !strings.EqualFold(fmt.Sprintf("%x", digest.Sum(nil)), expected) {
		return i18n.Error("resource.checksum_failed")
	}
	z, err := zip.NewReader(f, n)
	if err != nil {
		return i18n.Error("resource.archive_invalid", err)
	}
	for _, entry := range z.File {
		if !entry.FileInfo().IsDir() {
			for _, marker := range markers {
				if entry.Name == marker || kind == ShaderPacks && strings.HasPrefix(entry.Name, marker) {
					return ctx.Err()
				}
			}
		}
	}
	return i18n.Error("resource.archive_missing_marker", strings.Join(markers, i18n.Text("common.or_separator")))
}

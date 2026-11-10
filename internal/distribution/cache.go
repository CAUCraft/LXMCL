package distribution

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

// Cache stores verified files by digest. Root is the application cache directory.
// Its layout intentionally preserves the existing modpacks cache.
type Cache struct {
	Root   string
	Client *http.Client
}

// Ensure returns a file only after verifying its bytes, including cache hits.
// Client may customize transport and timeout; the source and redirect policy is fixed.
func (c Cache) Ensure(ctx context.Context, a Asset) (string, error) {
	if !a.Valid(MaxFileSize) {
		return "", i18n.Error("distribution.metadata_invalid")
	}
	if !filepath.IsAbs(c.Root) {
		return "", i18n.Error("distribution.cache_directory_required")
	}
	dir := filepath.Join(c.Root, "modpacks", a.Algorithm)
	path := filepath.Join(dir, strings.ToLower(a.Digest))
	if f, err := os.Open(path); err == nil {
		err = a.CopyVerified(ctx, io.Discard, f)
		f.Close()
		if err == nil {
			return path, nil
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	err := a.Fetch(ctx, c.Client, func(ctx context.Context, reader io.Reader) error {
		f, err := os.CreateTemp(dir, ".download-*")
		if err != nil {
			return err
		}
		defer os.Remove(f.Name())
		if err := errors.Join(a.CopyVerified(ctx, f, reader), f.Close()); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return os.Rename(f.Name(), path)
	})
	if err != nil {
		return "", err
	}
	return path, nil
}

package transfer

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

// Ensure reuses verified bytes or downloads a replacement into a temporary file.
// fetch owns sources and retries; consume must finish before fetch returns and
// must not be called concurrently. Failed attempts preserve the old cache file.
// Callers serialize writes to the same destination. unknownLimit only bounds
// artifacts with UnknownSize; known sizes use their declared length.
func (a Artifact) Ensure(ctx context.Context, file string, unknownLimit int64, fetch func(context.Context, string, func(context.Context, io.Reader) error) error) error {
	v, err := a.verification(unknownLimit)
	if err != nil {
		return i18n.Error("common.context_error", filepath.Base(file), err)
	}
	ok, err := v.checkFile(ctx, file)
	if ok || err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		return err
	}
	return fetch(ctx, a.URL, func(ctx context.Context, body io.Reader) error {
		f, err := os.CreateTemp(filepath.Dir(file), ".download-*")
		if err != nil {
			return err
		}
		defer os.Remove(f.Name())
		_, err = v.copy(ctx, f, body)
		if err = errors.Join(err, f.Close()); err != nil {
			if errors.Is(err, ErrMismatch) {
				return i18n.Error("common.context_error", filepath.Base(file), err)
			}
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return os.Rename(f.Name(), file)
	})
}

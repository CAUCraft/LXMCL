package transfer

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

type Artifact struct {
	Path string `json:"path"`
	URL  string `json:"url"`
	SHA  string `json:"sha1"`
	Size int64  `json:"size"`
}

// UnknownSize allows a missing length; the caller supplies a bound instead.
const UnknownSize = -1

var ErrMismatch = i18n.Error("download.checksum_mismatch")

func (a Artifact) Checksum() ([]byte, error) {
	sum, err := hex.DecodeString(a.SHA)
	if err != nil || len(sum) != sha1.Size {
		return nil, i18n.Error("download.sha1_invalid")
	}
	if a.Size < UnknownSize || a.Size == math.MaxInt64 {
		return nil, i18n.Error("download.file_size_invalid")
	}
	return sum, nil
}

func Check(ctx context.Context, file string, a Artifact, digest []byte, unknownLimit int64) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	st, err := os.Stat(file)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !st.Mode().IsRegular() {
		return false, i18n.Error("download.not_regular_file", file)
	}
	if a.Size == UnknownSize && st.Size() > unknownLimit || a.Size != UnknownSize && a.Size != st.Size() {
		return false, nil
	}
	f, err := os.Open(file)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()
	h := sha1.New()
	n, err := io.Copy(h, io.LimitReader(cancelReader{ctx, f}, st.Size()+1))
	return err == nil && n == st.Size() && bytes.Equal(h.Sum(nil), digest), err
}

func (a Artifact) Ensure(ctx context.Context, file string, unknownLimit int64, fetch func(context.Context, string, func(context.Context, io.Reader) error) error) error {
	digest, err := a.Checksum()
	if err != nil {
		return i18n.Error("common.context_error", filepath.Base(file), err)
	}
	ok, err := Check(ctx, file, a, digest, unknownLimit)
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
		h := sha1.New()
		limit := a.Size + 1
		if a.Size == UnknownSize {
			limit = unknownLimit + 1
		}
		n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(cancelReader{ctx, body}, limit))
		if err = errors.Join(err, f.Close()); err != nil {
			return err
		}
		if n >= limit || (a.Size != UnknownSize && n != a.Size) || !bytes.Equal(h.Sum(nil), digest) {
			return i18n.Error("common.context_error", filepath.Base(file), ErrMismatch)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return os.Rename(f.Name(), file)
	})
}

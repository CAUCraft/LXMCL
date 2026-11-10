package transfer

import (
	"bytes"
	"context"
	"crypto/sha1"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

// Check verifies the local file against this artifact's metadata. Missing or
// mismatched bytes return false, nil; invalid metadata, unsupported file types,
// I/O failures and cancellation return errors. unknownLimit is used only when
// Size is UnknownSize and must be nonnegative and less than math.MaxInt64.
func (a Artifact) Check(ctx context.Context, file string, unknownLimit int64) (bool, error) {
	v, err := a.verification(unknownLimit)
	if err != nil {
		return false, i18n.Error("common.context_error", filepath.Base(file), err)
	}
	return v.checkFile(ctx, file)
}

// artifactVerification keeps a validated digest and its size rules together.
// A fresh hash is created for every copy, including each fetch retry.
type artifactVerification struct {
	digest      []byte
	size, limit int64
}

func (a Artifact) verification(unknownLimit int64) (artifactVerification, error) {
	digest, err := a.Checksum()
	if err != nil {
		return artifactVerification{}, err
	}
	limit := a.Size
	if a.Size == UnknownSize {
		if unknownLimit < 0 || unknownLimit == math.MaxInt64 {
			return artifactVerification{}, i18n.Error("download.file_size_invalid")
		}
		limit = unknownLimit
	}
	return artifactVerification{digest: digest, size: a.Size, limit: limit}, nil
}

func (v artifactVerification) checkFile(ctx context.Context, file string) (bool, error) {
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
	if st.Size() > v.limit || v.size != UnknownSize && st.Size() != v.size {
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
	// Bound the local read to its observed size, including when metadata omits it.
	v.limit = st.Size()
	n, err := v.copy(ctx, io.Discard, f)
	if errors.Is(err, ErrMismatch) {
		return false, nil
	}
	return err == nil && n == st.Size(), err
}

func (v artifactVerification) copy(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	h := sha1.New()
	n, err := io.Copy(io.MultiWriter(dst, h), io.LimitReader(Reader(ctx, src), v.limit+1))
	if err != nil {
		return n, err
	}
	if err := ctx.Err(); err != nil {
		return n, err
	}
	if n > v.limit || v.size != UnknownSize && n != v.size || !bytes.Equal(h.Sum(nil), v.digest) {
		return n, ErrMismatch
	}
	return n, nil
}

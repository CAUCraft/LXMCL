package distribution

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"hash"
	"io"
	"net/url"
	"slices"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

// MaxFileSize bounds each distributed file, including standalone archives.
const MaxFileSize = 1 << 30

// Asset carries the size and digest supplied by a distribution platform.
type Asset struct {
	URLs              []string
	Size              int64
	Algorithm, Digest string
}

func (a Asset) hash() hash.Hash {
	switch a.Algorithm {
	case "sha512":
		return sha512.New()
	case "sha1":
		return sha1.New()
	case "md5":
		return md5.New()
	}
	return nil
}

func (a Asset) validChecksum(limit int64) bool {
	h := a.hash()
	digest, err := hex.DecodeString(a.Digest)
	return a.Size >= 0 && a.Size <= limit && h != nil && err == nil && len(digest) == h.Size()
}

// Valid requires a bounded size, a complete supported digest and an allowed source.
func (a Asset) Valid(limit int64) bool {
	return a.validChecksum(limit) && slices.ContainsFunc(a.URLs, func(raw string) bool {
		u, err := url.Parse(raw)
		return err == nil && AllowedURL(u)
	})
}

// CopyVerified copies at most Size+1 bytes and validates the complete file.
// The caller must discard the destination on error; this method does not publish it.
func (a Asset) CopyVerified(ctx context.Context, dst io.Writer, src io.Reader) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !a.validChecksum(MaxFileSize) {
		return i18n.Error("distribution.metadata_invalid")
	}
	h := a.hash()
	n, err := io.Copy(io.MultiWriter(dst, h), io.LimitReader(transfer.Reader(ctx, src), a.Size+1))
	if err != nil {
		return err
	}
	if n != a.Size || !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), a.Digest) {
		return i18n.Error("distribution.integrity_failed")
	}
	return ctx.Err()
}

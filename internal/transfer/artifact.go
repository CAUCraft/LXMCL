package transfer

import (
	"crypto/sha1"
	"encoding/hex"
	"math"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

// Artifact describes one SHA-1 verified Minecraft or Java file.
// Path is its metadata path; Check and Ensure take the local destination explicitly.
type Artifact struct {
	Path string `json:"path"`
	URL  string `json:"url"`
	SHA  string `json:"sha1"`
	Size int64  `json:"size"`
}

// UnknownSize allows a missing length; the caller supplies a bound instead.
const UnknownSize = -1

var ErrMismatch = i18n.Error("download.checksum_mismatch")

// Checksum validates the metadata and returns its digest for cache naming.
// File verification uses Check, which binds the digest to this artifact.
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

package content

import (
	"context"
	"fmt"
	"hash"
	"io"
	"os"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

// WriteVerified publishes a new resource after size, optional digest and archive
// checks. Callers must serialize resource changes for the same instance; enabled
// and disabled filenames cannot be checked atomically as a pair.
func WriteVerified(ctx context.Context, i instance.Instance, kind Kind, name string, in io.Reader, size int64, digest hash.Hash, expected string) error {
	return publishResource(ctx, i, kind, name, size, func(f *os.File) error {
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
		return nil
	})
}

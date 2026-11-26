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
	return writeVerifiedFile(ctx, i, kind, File{Name: name, Size: size, Enabled: true}, in, digest, expected)
}

func writeVerifiedFile(ctx context.Context, i instance.Instance, kind Kind, file File, in io.Reader, digest hash.Hash, expected string) error {
	return publishResource(ctx, i, kind, file, verifiedResourceWriter(ctx, in, file.Size, digest, expected))
}

func verifiedResourceWriter(ctx context.Context, in io.Reader, size int64, digest hash.Hash, expected string) func(*os.File) error {
	return func(f *os.File) error {
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
	}
}

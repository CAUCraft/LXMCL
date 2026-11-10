package content

import (
	"archive/zip"
	"context"
	"io"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

// ValidateArchive checks the resource format for the target instance, including
// its loader marker. Both newly imported files and existing downloads use it.
func ValidateArchive(ctx context.Context, i instance.Instance, kind Kind, src io.ReaderAt, size int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, markers, err := kind.Format(i)
	if err != nil {
		return err
	}
	if size <= 0 || size > MaxSize {
		return i18n.Error("resource.filename_or_size_invalid")
	}
	z, err := zip.NewReader(src, size)
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

package content

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
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

// ImportMany retains completed imports and reports individual failures without
// overwriting existing resources. Canceling stops before the next file.
func ImportMany(ctx context.Context, i instance.Instance, kind Kind, sources []string) (int, error) {
	completed := 0
	var failures []error
	for _, source := range sources {
		if err := ctx.Err(); err != nil {
			return completed, errors.Join(append(failures, err)...)
		}
		if err := Import(ctx, i, kind, source); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", filepath.Base(source), err))
		} else {
			completed++
		}
	}
	return completed, errors.Join(failures...)
}

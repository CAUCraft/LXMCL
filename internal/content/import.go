package content

import (
	"context"
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

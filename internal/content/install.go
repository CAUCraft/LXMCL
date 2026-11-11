package content

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"

	"github.com/CAUCraft/LXMCL/internal/distribution"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

// Install verifies and reuses an existing resource, or downloads a missing one.
// Conflicting or invalid existing files return an error and remain unchanged.
// The caller selects a compatible version and serializes changes to the target
// instance. A nil client uses distribution defaults; source policy stays fixed.
func Install(ctx context.Context, target instance.Instance, kind Kind, name string, asset distribution.Asset, client *http.Client) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateResource(target, kind, name, asset.Size); err != nil {
		return err
	}
	if !asset.Valid(MaxSize) {
		return i18n.Error("distribution.metadata_invalid")
	}
	if exists, err := existingResource(ctx, target, kind, name, asset); err != nil || exists {
		return err
	}
	return asset.Fetch(ctx, client, func(ctx context.Context, reader io.Reader) error {
		return publishResource(ctx, target, kind, name, asset.Size, func(file *os.File) error {
			return asset.CopyVerified(ctx, file, reader)
		})
	})
}

func existingResource(ctx context.Context, target instance.Instance, kind Kind, name string, asset distribution.Asset) (bool, error) {
	root, err := OpenDirectory(target, kind, false)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer root.Close()
	info, err := root.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() || info.Size() != asset.Size {
		return false, i18n.Error("resource.file_name_conflict", name)
	}
	source, err := root.Open(name)
	if err != nil {
		return false, err
	}
	defer source.Close()
	if err := asset.CopyVerified(ctx, io.Discard, source); err != nil {
		return false, i18n.Error("resource.existing_file_invalid", name, err)
	}
	if err := ValidateArchive(ctx, target, kind, source, asset.Size); err != nil {
		return false, err
	}
	return true, nil
}

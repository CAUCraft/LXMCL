package minecraft

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

func verifyArtifact(ctx context.Context, file fileDownload) error {
	digest, err := file.file.Checksum()
	if err != nil {
		return i18n.Error("common.context_error", filepath.Base(file.path), err)
	}
	ok, err := transfer.Check(ctx, file.path, file.file, digest, maxMetadata)
	if err != nil {
		return err
	}
	if !ok {
		return i18n.Error("install.file_unverified", filepath.Base(file.path))
	}
	return nil
}

func (g Runtime) verifyInstalled(ctx context.Context, i instance.Instance) (metadata, []string, error) {
	r, err := g.readReceipt(i.Version)
	if errors.Is(err, os.ErrNotExist) {
		err = i18n.Error("install.receipt_missing")
	}
	if err != nil {
		return metadata{}, nil, err
	}
	m, err := g.readVersion(i.Version, r.VersionSHA)
	if err != nil {
		return m, nil, err
	}
	files, cp, err := g.binaries(m)
	if err != nil {
		return m, nil, err
	}
	index, err := g.indexPath(m)
	if err != nil {
		return m, nil, err
	}
	// Check the asset index before trusting its references to other files.
	if err := verifyArtifact(ctx, fileDownload{index, m.AssetIndex.artifact}); err != nil {
		return m, nil, err
	}
	assets, err := g.assets(index)
	if err != nil {
		return m, nil, err
	}
	return m, cp, transfer.Batch(ctx, append(files, assets...), verifyArtifact, nil)
}

package launcher

import (
	"context"

	"github.com/CAUCraft/LXMCL/internal/saves"
)

func BackupsDir(root string, id int) string { return saves.BackupsDir(root, id) }

func BackupSaves(ctx context.Context, i Instance, dataRoot string) (string, error) {
	return saves.Backup(ctx, i.GameDir, dataRoot, i.ID)
}

func RestoreSaves(ctx context.Context, i Instance, source string) (int, error) {
	return saves.Restore(ctx, i.GameDir, source)
}

type SavePreview = saves.Preview

func InspectSaveArchive(ctx context.Context, source string) (SavePreview, error) {
	return saves.Inspect(ctx, source)
}

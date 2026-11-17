package launcher

import (
	"context"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/tasks"
)

// DuplicateWithContent publishes copied files before saving the captured draft.
// The caller serializes writes and prevents games from modifying the source.
func DuplicateWithContent(ctx context.Context, store Store, next State, source, target Instance, report func(Progress)) error {
	old, sourceExists := next.Find(source.ID)
	duplicate, targetExists := next.Find(target.ID)
	if !sourceExists || !targetExists || old != source || duplicate != target || source.ID == target.ID ||
		source.GameDir == target.GameDir || source.Version != target.Version || source.Loader != target.Loader || source.LoaderVersion != target.LoaderVersion {
		return i18n.Error("instance.duplicate_target_changed")
	}
	return tasks.Run(ctx, func(ctx context.Context, progress func(Progress)) error {
		return content.DuplicateSetup(ctx, source.GameDir, target.GameDir, func() error { return store.Save(next) }, progress)
	}, report)
}

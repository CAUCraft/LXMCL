package launcher

import (
	"context"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/i18n"
)

func (s Store) ImportPrism(ctx context.Context, next State, target Instance, p content.PrismInstance, worlds, options bool, report func(Progress)) error {
	return s.ImportPrismFiles(ctx, next, target, p, content.PrismFiles{Worlds: worlds, Options: options}, report)
}

func (s Store) ImportPrismFiles(ctx context.Context, next State, target Instance, p content.PrismInstance, selected content.PrismFiles, report func(Progress)) error {
	i, ok := next.Find(target.ID)
	if !ok || i != target || !i.Vanilla() || i.Version != p.Version || i.Launch != p.Launch {
		return i18n.Error("prism.changed")
	}
	if err := content.CheckSeparateDirectory(p.Source, s.Root); err != nil {
		return err
	}
	return content.CopyPrismFiles(ctx, p, target.GameDir, selected, func() error { return s.saveNewInstance(next, target) }, report)
}

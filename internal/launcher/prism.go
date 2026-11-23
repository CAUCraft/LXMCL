package launcher

import (
	"context"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/i18n"
)

func (s Store) ImportPrism(ctx context.Context, next State, target Instance, p content.PrismInstance, worlds, options bool, report func(Progress)) error {
	i, ok := next.Find(target.ID)
	if !ok || i != target || !i.Vanilla() || i.Version != p.Version || i.Launch != p.Launch {
		return i18n.Error("prism.changed")
	}
	if err := content.CheckSeparateDirectory(p.Source, s.Root); err != nil {
		return err
	}
	return content.CopyPrismInstance(ctx, p, target.GameDir, worlds, options, func() error { return s.saveNewInstance(next, target) }, report)
}

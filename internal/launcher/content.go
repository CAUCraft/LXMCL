package launcher

import (
	"context"

	"github.com/CAUCraft/LXMCL/internal/content"
)

type ContentKind = content.Kind
type ContentFile = content.File

const (
	ResourcePacks = content.ResourcePacks
	Mods          = content.Mods
	ShaderPacks   = content.ShaderPacks
)

func ListContent(i Instance, kind ContentKind) ([]ContentFile, error) {
	return content.List(i, kind)
}

func SetContentEnabled(i Instance, kind ContentKind, name string, enabled bool) error {
	return content.SetEnabled(i, kind, name, enabled)
}

func ImportContent(ctx context.Context, i Instance, kind ContentKind, source string) error {
	return content.Import(ctx, i, kind, source)
}

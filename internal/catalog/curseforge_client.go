package catalog

import (
	"context"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/modpack"
)

type curseForge struct {
	key    string
	client Client
}

func (c Client) curseForge(key string) curseForge {
	return curseForge{key: strings.TrimSpace(key), client: c}
}

// CurseForgeResolver resolves required manifest references using this transport.
// The key is retained only by the API adapter and never sent to file/icon requests.
func (c Client) CurseForgeResolver(key string) modpack.Resolver {
	// A function adapter keeps the captured credentials out of formatted options.
	return curseResolver(c.curseForge(key).Resolve)
}

type curseResolver func(context.Context, []modpack.Reference) ([]modpack.Download, error)

func (f curseResolver) Resolve(ctx context.Context, refs []modpack.Reference) ([]modpack.Download, error) {
	return f(ctx, refs)
}

func (cf curseForge) call(ctx context.Context, endpoint string, payload any, dst any) error {
	if cf.key == "" {
		return i18n.Error("curseforge.api_key_required")
	}
	return cf.client.json(ctx, "https://api.curseforge.com/v1/"+endpoint, cf.key, payload, dst)
}

package launcher

import (
	"context"
	"encoding/json"
	"io"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

func downloadJSON(ctx context.Context, d *downloads, source string, out any) error {
	return retry(ctx, func() error {
		res, err := d.get(ctx, source)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		body, err := io.ReadAll(io.LimitReader(res.Body, maxMetadata+1))
		if err != nil {
			return err
		}
		if len(body) > maxMetadata {
			return i18n.Error("metadata.response_too_large")
		}
		return json.Unmarshal(body, out)
	})
}

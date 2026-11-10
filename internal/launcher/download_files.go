package launcher

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/transfer"
)

type artifact = transfer.Artifact

func (d *downloads) file(ctx context.Context, file string, a artifact) error {
	return d.fileBound(ctx, file, a, maxMetadata)
}

func (d *downloads) fileBound(ctx context.Context, file string, a artifact, unknownLimit int64) error {
	return a.Ensure(ctx, file, unknownLimit, func(ctx context.Context, src string, consume func(context.Context, io.Reader) error) error {
		return d.fetch(ctx, src, func(ctx context.Context, resp *http.Response) error { return consume(ctx, resp.Body) })
	})
}

type fileDownload struct {
	path string
	file artifact
}

func (d *downloads) batch(ctx context.Context, stage string, files []fileDownload, report func(Progress)) error {
	report(Progress{Stage: stage, Total: len(files)})
	return transfer.Batch(ctx, files, func(ctx context.Context, file fileDownload) error {
		return d.file(ctx, file.path, file.file)
	}, func(done int) {
		report(Progress{Stage: stage, Done: done, Total: len(files)})
	})
}

func (d *downloads) fetchSHA1(ctx context.Context, source string) (string, error) {
	var sha string
	err := d.fetch(ctx, source+".sha1", func(ctx context.Context, resp *http.Response) error {
		data, err := io.ReadAll(io.LimitReader(transfer.Reader(ctx, resp.Body), 128))
		if err != nil {
			return err
		}
		s := strings.ToLower(strings.TrimSpace(string(data)))
		if _, err := (artifact{SHA: s, Size: unknownSize}).Checksum(); err != nil {
			return err
		}
		sha = s
		return nil
	})
	return sha, err
}

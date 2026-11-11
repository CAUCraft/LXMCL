package transfer

import (
	"context"
	"io"
)

type cancelReader struct {
	ctx context.Context
	r   io.Reader
}

func (r cancelReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func Reader(ctx context.Context, r io.Reader) io.Reader { return cancelReader{ctx, r} }

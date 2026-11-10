package transfer

import (
	"context"
	"sync"
	"sync/atomic"
)

func Batch[T any](ctx context.Context, files []T, work func(context.Context, T) error, progress func(int)) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	var next atomic.Int32
	var first error
	var once sync.Once
	var progressMu sync.Mutex
	done := 0
	for range min(12, len(files)) {
		wg.Go(func() {
			for ctx.Err() == nil {
				index := int(next.Add(1)) - 1
				if index >= len(files) {
					return
				}
				if err := work(ctx, files[index]); err != nil {
					once.Do(func() { first = err; cancel() })
					return
				}
				if progress != nil {
					progressMu.Lock()
					done++
					progress(done)
					progressMu.Unlock()
				}
			}
		})
	}
	wg.Wait()
	if first != nil {
		return first
	}
	return ctx.Err()
}

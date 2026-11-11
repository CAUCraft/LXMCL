package tasks

import (
	"context"
	"sync"
)

type Progress struct {
	Stage       string
	Done, Total int
}
type Work func(context.Context, func(Progress)) error

// Run owns progress delivery for one attempt. It serializes parallel reports,
// suppresses reports after completion, and checks cancellation before starting.
// The caller owns scheduling and retries; Work must honor ctx and join its workers.
func Run(ctx context.Context, work Work, report func(Progress)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var mu sync.Mutex
	finished := false
	defer func() { mu.Lock(); finished = true; mu.Unlock() }()
	return work(ctx, func(p Progress) {
		mu.Lock()
		defer mu.Unlock()
		if !finished && report != nil {
			report(p)
		}
	})
}

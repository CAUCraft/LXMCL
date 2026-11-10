package transfer

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

// HTTPError retains the server status, source label and requested retry delay.
type HTTPError struct {
	Status int
	Source string
	Delay  time.Duration
}

func (e *HTTPError) Error() string {
	if e.Status == http.StatusTooManyRequests {
		return i18n.Text("download.rate_limited")
	}
	return i18n.Text("download.http_status", e.Status, e.Source)
}

// RetryAfter interprets a server delay. Values over 30 seconds defer to user retry.
func RetryAfter(value string, now time.Time) time.Duration {
	if n, err := strconv.Atoi(value); err == nil && n >= 0 {
		// Any value beyond 30s is enough to defer to an explicit user Retry.
		return time.Duration(min(n, 31)) * time.Second
	}
	if date, err := http.ParseTime(value); err == nil {
		return max(0, date.Sub(now))
	}
	return time.Second
}

func Wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Retryable identifies transient transfer failures eligible for bounded automatic retry.
func Retryable(err error) bool {
	var response *HTTPError
	if errors.As(err, &response) {
		return response.Delay <= 30*time.Second && (response.Status == http.StatusRequestTimeout ||
			response.Status == http.StatusTooManyRequests || response.Status >= 500 && response.Status <= 599)
	}
	var operation *net.OpError
	var timeout net.Error
	return errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, ErrMismatch) ||
		errors.As(err, &operation) || (errors.As(err, &timeout) && timeout.Timeout())
}

// Retry makes at most three attempts with backoff and honors HTTPError.Delay.
// Cancellation interrupts both requests and the wait between attempts.
func Retry(ctx context.Context, fn func() error) error {
	var err error
	for attempt := range 3 {
		if err := ctx.Err(); err != nil {
			return err
		}
		err = fn()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err == nil || attempt == 2 || !Retryable(err) {
			return err
		}
		delay := time.Duration(1<<attempt) * 150 * time.Millisecond
		var response *HTTPError
		if errors.As(err, &response) {
			delay = max(delay, response.Delay)
		}
		if err := Wait(ctx, delay); err != nil {
			return err
		}
	}
	return err
}

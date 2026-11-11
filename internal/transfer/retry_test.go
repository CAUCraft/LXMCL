package transfer

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"testing/synctest"
	"time"
)

func TestRetryHonorsServerDelay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start, calls := time.Now(), 0
		err := Retry(t.Context(), func() error {
			calls++
			if calls == 1 {
				return &HTTPError{Status: http.StatusTooManyRequests, Delay: RetryAfter("2", time.Now())}
			}
			return nil
		})
		if err != nil || calls != 2 || time.Since(start) != 2*time.Second {
			t.Fatalf("retry = %v, %d calls after %v", err, calls, time.Since(start))
		}
	})
}

func TestRetryCancellationAndLimits(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		calls := 0
		err := Retry(ctx, func() error {
			calls++
			return &HTTPError{Status: http.StatusServiceUnavailable, Delay: 5 * time.Second}
		})
		if !errors.Is(err, context.DeadlineExceeded) || calls != 1 {
			t.Fatalf("cancellation retried: %d calls, %v", calls, err)
		}
		for _, tc := range []struct {
			err   error
			calls int
		}{
			{&HTTPError{Status: 429, Delay: RetryAfter("999999", time.Now())}, 1},
			{&HTTPError{Status: 404}, 1},
			{io.ErrUnexpectedEOF, 3},
		} {
			calls = 0
			err := Retry(t.Context(), func() error { calls++; return tc.err })
			if !errors.Is(err, tc.err) || calls != tc.calls {
				t.Fatalf("%v: got %d calls, %v", tc.err, calls, err)
			}
		}
	})
}

func TestRetryAfterHTTPDate(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if got := RetryAfter(now.Add(7*time.Second).Format(http.TimeFormat), now); got != 7*time.Second {
		t.Fatalf("HTTP date parsed as %v", got)
	}
	if got := RetryAfter(now.Add(-time.Second).Format(http.TimeFormat), now); got != 0 {
		t.Fatalf("past date parsed as %v", got)
	}
}

func TestRetryPreservesCompletedAttemptOnCancellation(t *testing.T) {
	for _, result := range []error{nil, io.ErrUnexpectedEOF} {
		ctx, cancel := context.WithCancel(t.Context())
		calls := 0
		err := Retry(ctx, func() error {
			calls++
			cancel()
			return result
		})
		want := error(nil)
		if result != nil {
			want = context.Canceled
		}
		if !errors.Is(err, want) || calls != 1 {
			t.Fatalf("attempt returned %v: got %v, %d calls; want %v", result, err, calls, want)
		}
	}
}

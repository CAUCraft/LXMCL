package distribution_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestModrinthStreamCannotRedirectToAnotherProvider(t *testing.T) {
	calls := 0
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return nil },
		Transport: transport(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 302, Header: http.Header{"Location": {"https://github.com/example/file.jar"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
		}),
	}
	err := asset("test").Fetch(t.Context(), client, func(context.Context, io.Reader) error {
		t.Fatal("cross-provider response reached the file writer")
		return nil
	})
	if err == nil || calls != 1 {
		t.Fatalf("redirect escaped policy: %v, calls %d", err, calls)
	}
}

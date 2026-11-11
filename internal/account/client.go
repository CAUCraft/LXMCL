package account

import (
	"net/http"
	"time"
)

// Client runs account protocols with bounded requests and no HTTP redirects.
// Its zero value uses the standard transport. A custom Transport can supply proxy
// policy or deterministic protocol fixtures without replacing global state.
// Configure it before use; the transport must support concurrent requests.
type Client struct{ Transport http.RoundTripper }

func (c Client) httpClient() *http.Client {
	return &http.Client{Transport: c.Transport, Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

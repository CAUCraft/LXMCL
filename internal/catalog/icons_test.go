package catalog_test

import (
	"bytes"
	"encoding/base64"
	"image"
	"io"
	"net/http"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/catalog"
)

type iconTransport func(*http.Request) (*http.Response, error)

func (f iconTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestStandaloneCatalogDecodesIcons(t *testing.T) {
	// Fixed bytes keep this test independent of encoder packages and their decoder registration.
	fixtures := map[string]string{
		"gif":  "R0lGODlhAgABAIcAAAAAAAAARAAAiAAAzABEAABERABEiABEzACIAACIRACIiACIzADMAADMRADMiADMzADd3REREQAAVQAAmQAA3QBVAABVVQBMmQBJ3QCZAACZTACZmQCT3QDdAADdSQDdkwDungDu7iIiIgAAZgAAqgAA7gBmAABmZgBVqgBP7gCqAACqVQCqqgCe7gDuAADuTwD/VQD/qgD//zMzMwAAdwAAuwAA/wB3AAB3dwBduwBV/wC7AAC7XQC7uwCq/wD/AEQAREQAiEQAzEREAEREREREiEREzESIAESIRESIiESIzETMAETMRETMiETMzEQAAFUAAFUAVUwAmUkA3VVVAFVVVUxMmUlJ3UyZAEyZTEyZmUmT3UndAEndSUndk0nd3U/u7mYAAGYAZlUAqk8A7mZmAGZmZlVVqk9P7lWqAFWqVVWqqk+e7k/uAE/uT0/unlX/qlX//3cAAHcAd10Au1UA/3d3AHd3d11du1VV/127AF27XV27u1Wq/1X/AFX/VYgAiIgAzIhEAIhERIhEiIhEzIiIAIiIRIiIiIiIzIjMAIjMRIjMiIjMzIgAAIgARJkATJkAmZMA3ZlMAJlMTJlMmZNJ3ZmZAJmZTJmZmZOT3ZPdAJPdSZPdk5Pd3ZkAAKoAAKoAVaoAqp4A7qpVAKpVVapVqp5P7qqqAKqqVaqqqp6e7p7uAJ7uT57unp7u7qr//7sAALsAXbsAu6oA/7tdALtdXbtdu6pV/7u7ALu7Xbu7u6qq/6r/AKr/Var/qswAzMxEAMxERMxEiMxEzMyIAMyIRMyIiMyIzMzMAMzMRMzMiMzMzMwAAMwARMwAiN0Ak90A3d1JAN1JSd1Jk91J3d2TAN2TSd2Tk92T3d3dAN3dSd3dk93d3d0AAN0ASe4AT+4Anu4A7u5PAO5PT+5Pnu5P7u6eAO6eT+6enu6e7u7uAO7uT+7unu7u7u4AAP8AAP8AVf8Aqv8A//9VAP9VVf9Vqv9V//+qAP+qVf+qqv+q////AP//Vf//qv///ywAAAAAAgABAAAIBQABAAgIADs=",
		"jpeg": "/9j/2wCEAAgGBgcGBQgHBwcJCQgKDBQNDAsLDBkSEw8UHRofHh0aHBwgJC4nICIsIxwcKDcpLDAxNDQ0Hyc5PTgyPC4zNDIBCQkJDAsMGA0NGDIhHCEyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMv/AABEIAAEAAgMBIgACEQEDEQH/xAGiAAABBQEBAQEBAQAAAAAAAAAAAQIDBAUGBwgJCgsQAAIBAwMCBAMFBQQEAAABfQECAwAEEQUSITFBBhNRYQcicRQygZGhCCNCscEVUtHwJDNicoIJChYXGBkaJSYnKCkqNDU2Nzg5OkNERUZHSElKU1RVVldYWVpjZGVmZ2hpanN0dXZ3eHl6g4SFhoeIiYqSk5SVlpeYmZqio6Slpqeoqaqys7S1tre4ubrCw8TFxsfIycrS09TV1tfY2drh4uPk5ebn6Onq8fLz9PX29/j5+gEAAwEBAQEBAQEBAQAAAAAAAAECAwQFBgcICQoLEQACAQIEBAMEBwUEBAABAncAAQIDEQQFITEGEkFRB2FxEyIygQgUQpGhscEJIzNS8BVictEKFiQ04SXxFxgZGiYnKCkqNTY3ODk6Q0RFRkdISUpTVFVWV1hZWmNkZWZnaGlqc3R1dnd4eXqCg4SFhoeIiYqSk5SVlpeYmZqio6Slpqeoqaqys7S1tre4ubrCw8TFxsfIycrS09TV1tfY2dri4+Tl5ufo6ery8/T19vf4+fr/2gAMAwEAAhEDEQA/APn+iiigD//Z",
		"png":  "iVBORw0KGgoAAAANSUhEUgAAAAIAAAABCAYAAAD0In+KAAAAFklEQVR4nAAJAPb/AgAAAAAAAAAAAwAAGwADK+C1DwAAAABJRU5ErkJggg==",
	}
	for format, encoded := range fixtures {
		t.Run(format, func(t *testing.T) {
			data, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				t.Fatal(err)
			}
			c := catalog.Client{Transport: iconTransport(func(req *http.Request) (*http.Response, error) {
				if req.Header.Get("x-api-key") != "" || req.Header.Get("Cookie") != "" {
					t.Fatal("icon request included credentials")
				}
				return &http.Response{StatusCode: 200, ContentLength: int64(len(data)), Body: io.NopCloser(bytes.NewReader(data))}, nil
			})}
			got, err := c.LoadIcon(t.Context(), "https://cdn.modrinth.com/icon."+format)
			if err != nil || got.Bounds() != image.Rect(0, 0, 96, 96) {
				t.Fatalf("icon = %v, %v", got, err)
			}
		})
	}
}

func TestCatalogIconRedirectStaysWithinApprovedSources(t *testing.T) {
	calls := 0
	c := catalog.Client{Transport: iconTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": {"https://example.com/untrusted.png"}}, Body: io.NopCloser(bytes.NewReader(nil))}, nil
	})}
	if _, err := c.LoadIcon(t.Context(), "https://cdn.modrinth.com/icon.png"); err == nil || calls != 1 {
		t.Fatalf("unexpected redirect: %d, %v", calls, err)
	}
}

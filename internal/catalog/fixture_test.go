package catalog

import (
	"bytes"
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func fixtureResponse(status int, data []byte) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), ContentLength: int64(len(data)), Body: io.NopCloser(bytes.NewReader(data))}
}

func jsonResponse(t *testing.T, value any) *http.Response {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return fixtureResponse(http.StatusOK, b)
}

func versionFixture(id string) PackVersion {
	f := PackFile{URL: "https://cdn.modrinth.com/" + id + ".jar", Filename: id + ".jar", Size: 4, Primary: true}
	f.Hashes.SHA512 = fmt.Sprintf("%x", sha512.Sum512([]byte("file")))
	return PackVersion{ID: id, ProjectID: "library", Loaders: []string{"neoforge"}, GameVersions: []string{"1.21.1"}, Files: []PackFile{f}}
}

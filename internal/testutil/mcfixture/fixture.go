// Package mcfixture provides deterministic Minecraft files for backend integration tests.
package mcfixture

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/transfer"
)

const ManifestURL = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"

type Version struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	URL  string `json:"url"`
	SHA  string `json:"sha1"`
}
type Fixture struct {
	Files   map[string][]byte
	Hits    map[string]int
	mu      sync.Mutex
	Asset   transfer.Artifact
	Version Version
}

func New(t *testing.T) *Fixture {
	t.Helper()
	f := &Fixture{Files: map[string][]byte{}, Hits: map[string]int{}}
	add := func(src string, data []byte) transfer.Artifact {
		sum := sha1.Sum(data)
		f.Files[src] = data
		return transfer.Artifact{URL: src, SHA: hex.EncodeToString(sum[:]), Size: int64(len(data))}
	}
	marshal := func(value any) []byte {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	client := add("https://piston-data.mojang.com/client.jar", []byte("client"))
	library := add("https://libraries.minecraft.net/example/library.jar", []byte("library"))
	library.Path = "example/library.jar"
	assetBody := []byte("texture")
	hash := sha1.Sum(assetBody)
	digest := hex.EncodeToString(hash[:])
	f.Asset = add("https://resources.download.minecraft.net/"+digest[:2]+"/"+digest, assetBody)
	entry := map[string]any{"hash": digest, "size": len(assetBody)}
	index := add("https://piston-meta.mojang.com/index.json", marshal(map[string]any{
		"objects": map[string]any{"texture/a": entry, "texture/b": entry},
	}))
	libraryEntry := map[string]any{"downloads": map[string]transfer.Artifact{"artifact": library}}
	version := add("https://piston-meta.mojang.com/version.json", marshal(map[string]any{
		"id": "1.21.1", "type": "release", "mainClass": "test.Main",
		"arguments": map[string][]string{
			"jvm":  {"-cp", "${classpath}"},
			"game": {"--username", "${auth_player_name}", "--accessToken", "${auth_access_token}", "--userType", "${user_type}"},
		},
		"javaVersion": map[string]int{"majorVersion": 21},
		"downloads":   map[string]transfer.Artifact{"client": client},
		"assetIndex":  map[string]any{"id": "17", "url": index.URL, "size": index.Size, "sha1": index.SHA},
		"libraries":   []any{libraryEntry, libraryEntry},
	}))
	f.Version = Version{ID: "1.21.1", Type: "release", URL: version.URL, SHA: version.SHA}
	add(ManifestURL, marshal(map[string][]Version{"versions": {f.Version}}))
	return f
}

func (f *Fixture) RoundTrip(req *http.Request) (*http.Response, error) {
	src := req.URL.String()
	f.mu.Lock()
	f.Hits[src]++
	f.mu.Unlock()
	data, ok := f.Files[src]
	if !ok {
		return Response(http.StatusNotFound, nil), nil
	}
	return Response(http.StatusOK, data), nil
}

func Response(code int, data []byte) *http.Response {
	return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(data)), ContentLength: int64(len(data))}
}

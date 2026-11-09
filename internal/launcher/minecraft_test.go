package launcher

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type installFixture struct {
	files   map[string][]byte
	hits    map[string]int
	mu      sync.Mutex
	asset   artifact
	version Version
}

func newInstallFixture(t *testing.T) *installFixture {
	t.Helper()
	f := &installFixture{files: map[string][]byte{}, hits: map[string]int{}}
	add := func(src string, data []byte) artifact {
		sum := sha1.Sum(data)
		f.files[src] = data
		return artifact{URL: src, SHA: hex.EncodeToString(sum[:]), Size: int64(len(data))}
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
	f.asset = add("https://resources.download.minecraft.net/"+digest[:2]+"/"+digest, assetBody)
	entry := map[string]any{"hash": digest, "size": len(assetBody)}
	index := add("https://piston-meta.mojang.com/index.json", marshal(map[string]any{
		"objects": map[string]any{"texture/a": entry, "texture/b": entry},
	}))
	libraryEntry := map[string]any{"downloads": map[string]artifact{"artifact": library}}
	version := add("https://piston-meta.mojang.com/version.json", marshal(map[string]any{
		"id": "1.21.1", "type": "release", "mainClass": "test.Main",
		"arguments":   map[string][]string{"jvm": {"-cp", "${classpath}"}},
		"javaVersion": map[string]int{"majorVersion": 21},
		"downloads":   map[string]artifact{"client": client},
		"assetIndex":  map[string]any{"id": "17", "url": index.URL, "size": index.Size, "sha1": index.SHA},
		"libraries":   []any{libraryEntry, libraryEntry},
	}))
	f.version = Version{ID: "1.21.1", Type: "release", URL: version.URL, SHA: version.SHA}
	add(manifestURL, marshal(map[string][]Version{"versions": {f.version}}))
	return f
}

func (f *installFixture) request(req *http.Request) (*http.Response, error) {
	src := req.URL.String()
	f.mu.Lock()
	f.hits[src]++
	f.mu.Unlock()
	data, ok := f.files[src]
	if !ok {
		return fixtureResponse(http.StatusNotFound, nil), nil
	}
	return fixtureResponse(http.StatusOK, data), nil
}

func fixtureResponse(code int, data []byte) *http.Response {
	return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(data))}
}

func TestInstallRepairsSameSizeCorruption(t *testing.T) {
	f := newInstallFixture(t)
	d := &downloads{http: &http.Client{Transport: transportFunc(f.request)}}
	g := Game{Cache: t.TempDir()}
	i := Instance{Version: f.version.ID, GameDir: t.TempDir()}
	ctx := t.Context()
	var progresses []Progress
	if err := g.install(ctx, i, func(p Progress) { progresses = append(progresses, p) }, d); err != nil {
		t.Fatal(err)
	}
	if !g.Installed(i) {
		t.Fatal("completed install has no valid receipt")
	}
	if len(progresses) != 6 || progresses[2].Done != 1 || progresses[3].Done != 2 ||
		progresses[5].Done != 1 || progresses[5].Total != 1 {
		t.Fatalf("progress should include deduplicated libraries and assets in order: %+v", progresses)
	}
	if _, cp, err := g.verifyInstalled(ctx, i); err != nil || len(cp) != 2 {
		t.Fatalf("verifyInstalled = %v, %v", cp, err)
	}
	object := filepath.Join(g.Cache, "assets", "objects", f.asset.SHA[:2], f.asset.SHA)
	if err := os.WriteFile(object, []byte("garbage"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Start(ctx, i, Settings{Player: "Player", Memory: 4096}); !errors.Is(err, ErrRepairRequired) {
		t.Fatalf("same-size corrupt asset should block starting the client: %v", err)
	}
	if err := g.install(ctx, i, nil, d); err != nil {
		t.Fatal(err)
	}
	for src, count := range f.hits {
		want := 1
		if src == f.asset.URL || src == manifestURL {
			want = 2
		}
		if count != want {
			t.Errorf("%s requested %d times, want %d", src, count, want)
		}
	}
	if b, err := os.ReadFile(object); err != nil || string(b) != "texture" {
		t.Fatalf("repair did not replace corrupt asset: %q, %v", b, err)
	}
	if err := writeFile(filepath.Join(g.versionDir(i.Version), "verified"), []byte("1\n")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := g.verifyInstalled(ctx, i); err != nil {
		t.Fatalf("legacy cache must remain usable: %v", err)
	}
	if _, err := g.Start(ctx, i, Settings{Player: "Player", Memory: 4096, JavaPath: filepath.Join(t.TempDir(), "missing-java")}); !errors.Is(err, ErrJavaRequired) {
		t.Fatalf("missing Java should report an actionable launch error: %v", err)
	}
	i.Launch = LaunchOptions{JavaMode: JavaCustom, JavaPath: filepath.Join(t.TempDir(), "missing-instance-java")}
	if _, minimum, err := g.CheckJava(ctx, i, "global-java"); minimum != 21 || err == nil || !strings.Contains(err.Error(), "missing-instance-java") {
		t.Fatalf("Java check ignored the instance executable or version metadata: minimum %d, %v", minimum, err)
	}
}

func TestClientArgsRespectInstanceDisplayMode(t *testing.T) {
	var m metadata
	raw := `["--username", "${auth_player_name}",
		{"rules":[{"action":"allow","features":{"has_custom_resolution":true}}],
		 "value":["--width","${resolution_width}","--height","${resolution_height}"]},
		{"rules":[{"action":"allow","features":{"has_quick_plays_support":true}}],
		 "value":["--quickPlayPath","${unsupported_path}"]}]`
	if err := json.Unmarshal([]byte(raw), &m.Arguments.Game); err != nil {
		t.Fatal(err)
	}
	base := []string{"--username", "Player"}
	cases := []struct {
		opts LaunchOptions
		want []string
	}{
		{LaunchOptions{}, base},
		{LaunchOptions{Display: DisplayResolution, Width: 1920, Height: 1080}, []string{"--username", "Player", "--width", "1920", "--height", "1080"}},
		{LaunchOptions{Display: DisplayFullscreen}, []string{"--username", "Player", "--fullscreen"}},
	}
	for _, tc := range cases {
		args, err := clientArgs(m, map[string]string{"auth_player_name": "Player"}, tc.opts, "")
		if err != nil || !slices.Equal(args, tc.want) {
			t.Errorf("display %q: args = %v, error = %v; want %v", tc.opts.Display, args, err, tc.want)
		}
	}
}

type canceledBody struct {
	ctx     context.Context
	started chan<- struct{}
}

func (r *canceledBody) Read(p []byte) (int, error) {
	if r.started != nil {
		close(r.started)
		r.started = nil
		return copy(p, "part"), nil
	}
	<-r.ctx.Done()
	return 0, r.ctx.Err()
}

func (*canceledBody) Close() error { return nil }

func TestCanceledInstallKeepsOnlyCompleteFiles(t *testing.T) {
	f := newInstallFixture(t)
	started := make(chan struct{})
	d := &downloads{http: &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() == f.asset.URL {
			return &http.Response{StatusCode: http.StatusOK, Body: &canceledBody{ctx: req.Context(), started: started}}, nil
		}
		return f.request(req)
	})}}
	g := Game{Cache: t.TempDir()}
	i := Instance{Version: f.version.ID, GameDir: t.TempDir()}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- g.install(ctx, i, nil, d) }()
	select {
	case <-started:
	case err := <-done:
		t.Fatalf("install stopped before asset download: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("asset download never started")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("install cancellation = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("install did not stop after cancellation")
	}
	if g.Installed(i) {
		t.Fatal("partial installation was marked verified")
	}
	objectDir := filepath.Join(g.Cache, "assets", "objects", f.asset.SHA[:2])
	if entries, err := os.ReadDir(objectDir); err != nil || len(entries) != 0 {
		t.Fatalf("partial download leaked: %v / %v", entries, err)
	}
	d.http.Transport = transportFunc(f.request)
	if err := g.install(t.Context(), i, nil, d); err != nil {
		t.Fatal(err)
	}
	if f.hits[f.version.URL] != 1 || f.hits[f.asset.URL] != 1 || !g.Installed(i) {
		t.Fatalf("retry did not reuse complete files: %v", f.hits)
	}
}

func TestDownloadRateLimitAndTransientFailure(t *testing.T) {
	f := newInstallFixture(t)
	for _, test := range []struct {
		name       string
		statuses   []int
		retryAfter string
		wantError  string
	}{
		{name: "rate limit and gateway failure", statuses: []int{429, 502, 200}, retryAfter: "0"},
		{name: "long rate limit stops", statuses: []int{429}, retryAfter: "60", wantError: "HTTP 429"},
		{name: "not found does not retry", statuses: []int{404}, wantError: "HTTP 404"},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			d := &downloads{http: &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if calls > len(test.statuses) {
					return nil, fmt.Errorf("unexpected retry")
				}
				resp := fixtureResponse(test.statuses[calls-1], f.files[req.URL.String()])
				resp.Header.Set("Retry-After", test.retryAfter)
				return resp, nil
			})}}
			path := filepath.Join(t.TempDir(), "asset")
			err := d.file(t.Context(), path, f.asset)
			if (test.wantError == "" && err != nil) || (test.wantError != "" && (err == nil || !strings.Contains(err.Error(), test.wantError))) {
				t.Fatalf("file = %v; want error containing %q", err, test.wantError)
			}
			if calls != len(test.statuses) {
				t.Fatalf("requests = %d, want %d", calls, len(test.statuses))
			}
		})
	}
}

func TestRateLimitCooldownIsSharedAndCancelable(t *testing.T) {
	f := newInstallFixture(t)
	hits := 0
	d := &downloads{http: &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		hits++
		resp := fixtureResponse(http.StatusTooManyRequests, nil)
		resp.Header.Set("Retry-After", "10")
		return resp, nil
	})}}
	_, err := d.get(t.Context(), f.asset.URL)
	if err == nil {
		t.Fatal("429 returned as a usable response")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = d.get(ctx, "https://resources.download.minecraft.net/another")
	if !errors.Is(err, context.Canceled) || hits != 1 {
		t.Fatalf("second URL on same host should honor the shared wait: %v, hits=%d", err, hits)
	}
}

func TestGameArgsDoesNotExpandPlaceholdersInValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "world with ${name} in the path")
	raw := []json.RawMessage{
		json.RawMessage(`"--gameDir=${dir}"`),
		json.RawMessage(`{"rules":[{"action":"allow","features":{"is_demo_user":true}}],"value":["--demo","${unset}"]}`),
		json.RawMessage(`"--username=${name}"`),
	}
	args, err := gameArgs(raw, map[string]string{"dir": path, "name": "Player"}, nil)
	if err != nil || len(args) != 2 || args[0] != "--gameDir="+path || args[1] != "--username=Player" {
		t.Fatalf("unexpected Java argument list: %v / %v", args, err)
	}
}

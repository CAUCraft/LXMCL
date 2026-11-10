package minecraft

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/CAUCraft/LXMCL/internal/account"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

func TestInstallRepairsSameSizeCorruption(t *testing.T) {
	f := newInstallFixture(t)
	d := &downloads{http: &http.Client{Transport: transportFunc(f.RoundTrip)}}
	g := Runtime{Cache: t.TempDir()}
	i := instance.Instance{Version: f.Version.ID, GameDir: t.TempDir()}
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
	object := filepath.Join(g.Cache, "assets", "objects", f.Asset.SHA[:2], f.Asset.SHA)
	if err := os.WriteFile(object, []byte("garbage"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Start(ctx, i, offlineLaunch(t, instance.Settings{Player: "Player", Memory: 4096}), nil); !errors.Is(err, ErrRepairRequired) {
		t.Fatalf("same-size corrupt asset should block starting the client: %v", err)
	}
	if err := g.install(ctx, i, nil, d); err != nil {
		t.Fatal(err)
	}
	for src, count := range f.Hits {
		want := 1
		if src == f.Asset.URL || src == manifestURL {
			want = 2
		}
		if count != want {
			t.Errorf("%s requested %d times, want %d", src, count, want)
		}
	}
	if b, err := os.ReadFile(object); err != nil || string(b) != "texture" {
		t.Fatalf("repair did not replace corrupt asset: %q, %v", b, err)
	}
	if err := storage.WriteFile(filepath.Join(g.versionDir(i.Version), "verified"), []byte("1\n")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := g.verifyInstalled(ctx, i); err != nil {
		t.Fatalf("legacy cache must remain usable: %v", err)
	}
	if _, err := g.Start(ctx, i, offlineLaunch(t, instance.Settings{Player: "Player", Memory: 4096, JavaPath: filepath.Join(t.TempDir(), "missing-java")}), nil); !errors.Is(err, ErrJavaRequired) {
		t.Fatalf("missing Java should report an actionable launch error: %v", err)
	}
	i.Launch = instance.LaunchOptions{JavaMode: instance.JavaCustom, JavaPath: filepath.Join(t.TempDir(), "missing-instance-java")}
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
		opts instance.LaunchOptions
		want []string
	}{
		{instance.LaunchOptions{}, base},
		{instance.LaunchOptions{Display: instance.DisplayResolution, Width: 1920, Height: 1080}, []string{"--username", "Player", "--width", "1920", "--height", "1080"}},
		{instance.LaunchOptions{Display: instance.DisplayFullscreen}, []string{"--username", "Player", "--fullscreen"}},
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
		if req.URL.String() == f.Asset.URL {
			return &http.Response{StatusCode: http.StatusOK, Body: &canceledBody{ctx: req.Context(), started: started}}, nil
		}
		return f.RoundTrip(req)
	})}}
	g := Runtime{Cache: t.TempDir()}
	i := instance.Instance{Version: f.Version.ID, GameDir: t.TempDir()}
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
	objectDir := filepath.Join(g.Cache, "assets", "objects", f.Asset.SHA[:2])
	if entries, err := os.ReadDir(objectDir); err != nil || len(entries) != 0 {
		t.Fatalf("partial download leaked: %v / %v", entries, err)
	}
	d.http.Transport = transportFunc(f.RoundTrip)
	if err := g.install(t.Context(), i, nil, d); err != nil {
		t.Fatal(err)
	}
	if f.Hits[f.Version.URL] != 1 || f.Hits[f.Asset.URL] != 1 || !g.Installed(i) {
		t.Fatalf("retry did not reuse complete files: %v", f.Hits)
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
				resp := fixtureResponse(test.statuses[calls-1], f.Files[req.URL.String()])
				resp.Header.Set("Retry-After", test.retryAfter)
				return resp, nil
			})}}
			path := filepath.Join(t.TempDir(), "asset")
			err := d.file(t.Context(), path, f.Asset)
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
	_, err := d.get(t.Context(), f.Asset.URL)
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

func offlineLaunch(t *testing.T, settings instance.Settings) LaunchOptions {
	t.Helper()
	identity, err := account.OfflineIdentity(settings.Player)
	if err != nil {
		t.Fatal(err)
	}
	return LaunchOptions{Settings: settings, Identity: identity}
}

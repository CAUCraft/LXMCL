package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
)

type modNotesTransport func(*http.Request) (*http.Response, error)

func (f modNotesTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestModUpdateReleaseNotes(t *testing.T) {
	a, tt := testDesktop(t)
	a.ctx = t.Context()
	a.state.Instances[1].Loader, a.state.Instances[1].LoaderVersion = "Fabric", "0.16.14"
	i := a.state.Instances[1]
	a.inspect(i)
	a.detailPage = detailContent
	file := launcher.ContentFile{Name: "atlas.jar", Enabled: true, Size: 3200}
	m := launcher.InstalledMod{File: file, Digest: "synthetic-file"}
	m.Title = "Atlas"
	m.Current = launcher.PackVersion{ID: "initial", ProjectID: "atlas", Number: "1.0"}
	m.Update = launcher.PackVersion{ID: "cached", ProjectID: "atlas", Number: "2.0"}
	m.Update.Changelog = json.RawMessage(`"# 更新内容\n\n- 改进地图缩放和图层切换\n- 保留自定义地图标记\n\n[文档](https://example.invalid/docs)"`)
	m.Unconfirmed = []launcher.ModDependency{{ProjectID: "render"}}
	a.content = contentView{id: i.ID, revision: i.Revision, tab: 1, files: []launcher.ContentFile{file},
		mods: map[string]launcher.InstalledMod{file.Name: m}, selected: map[string]bool{file.Name: true}}
	var requests atomic.Int32
	a.game.Transport = modNotesTransport(func(req *http.Request) (*http.Response, error) {
		n := requests.Add(1)
		if req.Method != http.MethodGet || req.URL.String() != "https://api.modrinth.com/v2/version/next" ||
			req.ContentLength != 0 || req.Header.Get("Authorization") != "" || req.Header.Get("x-api-key") != "" {
			t.Error("unexpected notes request", req.Method, req.URL)
			return nil, errors.New("unexpected notes request")
		}
		status, body := http.StatusOK, `{"id":"next","project_id":"atlas","changelog":null}`
		if n == 1 {
			status, body = http.StatusNotFound, ""
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	tt.SetSize(820, 530)
	tt.Frame()
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(label, err, tt.Texts())
		}
		tt.Frame()
	}
	wait := func(label string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for !tt.HasText(label) && time.Now().Before(deadline) {
			time.Sleep(2 * time.Millisecond)
			tt.Frame()
		}
		if !tt.HasText(label) {
			t.Fatal("missing", label, tt.Texts())
		}
	}
	const open = "查看发布说明 atlas.jar"
	const back = "返回模组列表"
	if requests.Load() != 0 {
		t.Fatal("rendering mod list requested release notes")
	}
	click(open)
	wait("改进地图缩放和图层切换")
	wait("[文档](https://example.invalid/docs)")
	for _, dark := range []bool{false, true} {
		tt.SetDark(dark)
		tt.Advance(400 * time.Millisecond)
		tt.Frame()
		b, ok := tt.Find(back)
		if !ok || b.X < 0 || b.Y < 42 || b.X+b.W > 820 || b.Y+b.H > 530 || tt.HasText("更新选中 (1)") {
			t.Fatal("inaccessible notes navigation or update action in notes", b, tt.Texts())
		}
		name := "mod-update-notes-light"
		if dark {
			name = "mod-update-notes-dark"
		}
		capture(t, tt, name)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if requests.Load() != 0 || !tt.HasText("更新选中 (1)") || a.content.modNotes != nil {
		t.Fatal("cached notes requested data or altered the selection", tt.Texts())
	}

	m.Update.ID, m.Update.Changelog = "next", nil // Same display number, different candidate.
	a.content.mods[file.Name] = m
	tt.Frame()
	click(open)
	wait("无法读取发布说明")
	click("重试")
	wait("此版本没有发布说明。")
	click(back)
	click(open)
	wait("此版本没有发布说明。")
	click(back)
	if requests.Load() != 2 || a.state.Selected != 1 || a.busy || len(a.history) != 0 ||
		!tt.HasText("更新选中 (1)") {
		t.Fatal("notes changed the launch target, started a task or re-fetched saved metadata", requests.Load(), tt.Texts())
	}
}

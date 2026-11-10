package launcher

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha512"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/distribution"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/modpack"
)

func packArchive(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	for name, body := range entries {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func packFile(t *testing.T, name string, entries map[string]string) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(file, packArchive(t, entries), 0600); err != nil {
		t.Fatal(err)
	}
	return file
}

func packResponse(data []byte) *http.Response {
	resp := fixtureResponse(http.StatusOK, data)
	resp.ContentLength = int64(len(data))
	return resp
}

func packState(t *testing.T, info ModpackInfo) (Store, State, State, Instance) {
	t.Helper()
	store := Store{Root: t.TempDir()}
	old, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := old.Create("Existing", "1.21.1"); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(old); err != nil {
		t.Fatal(err)
	}
	next := old.Clone()
	if err := next.Create("Imported", info.Minecraft); err != nil {
		t.Fatal(err)
	}
	if info.Loader != LoaderVanilla {
		if err := next.ConfigureModLoader(next.Selected, 1, info.Loader, info.LoaderVersion); err != nil {
			t.Fatal(err)
		}
	}
	i, _ := next.Find(next.Selected)
	next.Selected = old.Selected
	return store, old, next, i
}

func TestModpackInstallIsIndependentAndVerified(t *testing.T) {
	const dependency = "test mod bytes"
	const uri = "https://cdn.modrinth.com/data/test/dependency.jar"
	manifest := fmt.Sprintf(`{"formatVersion":1,"game":"minecraft","name":"Imported","versionId":"1.0",
		"dependencies":{"minecraft":"1.21.1"},"files":[
		{"path":"mods/dependency.jar","fileSize":%d,"hashes":{"sha512":"%x"},"downloads":[%q]},
		{"path":"mods/optional.jar","env":{"client":"optional"}},
		{"path":"mods/server.jar","env":{"client":"unsupported"}}]}`, len(dependency), sha512.Sum512([]byte(dependency)), uri)
	file := packFile(t, "demo.mrpack", map[string]string{
		"modrinth.index.json": manifest, "overrides/config/options.txt": "shared",
		"client-overrides/config/options.txt": "client", "server-overrides/config/server.txt": "server",
	})
	ctx := t.Context()
	pack, err := PrepareLocalModpack(ctx, file)
	if err != nil {
		t.Fatal(err)
	}
	if info := pack.Info(); info.Downloads != 1 || info.Overrides != 2 || info.Minecraft != "1.21.1" {
		t.Fatalf("unexpected plan: %+v", info)
	}
	store, old, next, i := packState(t, pack.Info())
	g := Game{Cache: t.TempDir()}
	fixture := newInstallFixture(t)
	gameHTTP := &http.Client{Transport: transportFunc(fixture.request)}
	install := func(ctx context.Context, work Instance, report func(Progress)) error {
		return g.install(ctx, work, report, &downloads{http: gameHTTP})
	}
	data := strings.Repeat("x", len(dependency))
	client := &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != uri {
			return nil, fmt.Errorf("unexpected download: %s", req.URL)
		}
		return packResponse([]byte(data)), nil
	})}
	verifyNoInstance := func() {
		t.Helper()
		loaded, err := store.Load()
		if err != nil || len(loaded.Instances) != 1 || loaded.Selected != old.Selected || loaded.NextID != old.NextID {
			t.Fatalf("failure published a new instance: %+v, %v", loaded, err)
		}
		if entries, err := os.ReadDir(next.Settings.GameRoot); err != nil || len(entries) != 0 {
			t.Fatalf("failure left files in the game root: %v, %v", entries, err)
		}
	}
	err = g.installModpack(ctx, store, next, i, pack, curseForge{}, client, install, nil)
	if err == nil || !strings.Contains(err.Error(), i18n.Text("distribution.integrity_failed")) {
		t.Fatalf("corrupt dependency accepted: %v", err)
	}
	verifyNoInstance()
	data = dependency
	canceled, cancel := context.WithCancel(ctx)
	err = g.installModpack(canceled, store, next, i, pack, curseForge{}, client, install, func(p Progress) {
		if p.Stage == i18n.Text("progress.extract_modpack_overrides") {
			cancel()
		}
	})
	cancel()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("install did not cancel: %v", err)
	}
	verifyNoInstance()
	// Exercise failure after the staged directory has been filled and validated.
	unwritable := Store{Root: file}
	if err := g.installModpack(ctx, unwritable, next, i, pack, curseForge{}, client, install, nil); err == nil {
		t.Fatal("install with an invalid store unexpectedly succeeded")
	}
	verifyNoInstance()
	if err := g.installModpack(ctx, store, next, i, pack, curseForge{}, client, install, nil); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil || len(loaded.Instances) != 2 || loaded.Selected != old.Selected || !g.Installed(i) {
		t.Fatalf("new instance or original selection missing: %+v, %v", loaded, err)
	}
	for path, want := range map[string]string{"mods/dependency.jar": dependency, "config/options.txt": "client"} {
		b, err := os.ReadFile(filepath.Join(i.GameDir, path))
		if err != nil || string(b) != want {
			t.Fatalf("%s = %q, %v", path, b, err)
		}
	}
	for _, path := range []string{"mods/optional.jar", "mods/server.jar", "config/server.txt"} {
		if _, err := os.Stat(filepath.Join(i.GameDir, path)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("non-client file installed: %s, %v", path, err)
		}
	}
	if err := g.installModpack(ctx, store, next, i, pack, curseForge{}, client, install, nil); err == nil {
		t.Fatal("install overwrote the existing instance directory")
	}
}

func TestModpackRejectsUnsafeArchives(t *testing.T) {
	for _, entry := range []string{"overrides/../../outside", "overrides/config\\outside", "overrides/AUX.txt",
		"client-overrides/config/OPTIONS.txt", "overrides/config/options.txt/child"} {
		t.Run(entry, func(t *testing.T) {
			file := packFile(t, "unsafe.mrpack", map[string]string{
				"modrinth.index.json":          `{"formatVersion":1,"game":"minecraft","dependencies":{"minecraft":"1.21.1"}}`,
				"overrides/config/options.txt": "original", entry: "invalid",
			})
			if _, err := PrepareLocalModpack(t.Context(), file); err == nil {
				t.Fatalf("accepted %q", entry)
			}
		})
	}
	file := packFile(t, "unsupported.zip", map[string]string{
		"manifest.json": `{"manifestType":"minecraftModpack","manifestVersion":1,
			"minecraft":{"version":"1.21.1","modLoaders":[{"id":"unknown-0.28.1","primary":true}]}}`,
	})
	if _, err := PrepareLocalModpack(t.Context(), file); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("did not reject unsupported runtime: %v", err)
	}
}

func TestCurseForgeCatalogAndImport(t *testing.T) {
	const apiKey = "mock-curseforge-key"
	const dependency = "fabric content"
	const uri = "https://edge.forgecdn.net/files/1/2/example.jar"
	const archiveURL = "https://edge.forgecdn.net/files/1/3/pack.zip"
	entries := map[string]string{
		"manifest.json": `{"manifestType":"minecraftModpack","manifestVersion":1,"name":"Curse pack","version":"1",
			"minecraft":{"version":"1.21.1","modLoaders":[{"id":"forge-52.1.14"},{"id":"neoforge-21.1.200","primary":true}]},
			"files":[{"projectID":30,"fileID":40,"required":true},{"projectID":41,"fileID":42,"required":false}],
			"overrides":"overrides"}`,
		"overrides/options.txt": "cf settings",
	}
	archive := packArchive(t, entries)
	allow := false
	api := &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("x-api-key") != apiKey || req.URL.Host != "api.curseforge.com" {
			return nil, errors.New("API key sent to unexpected request")
		}
		var json string
		switch req.URL.Path {
		case "/v1/mods/search":
			if q := req.URL.Query(); q.Get("classId") != "4471" || q.Get("gameVersion") != "1.21.1" {
				return nil, fmt.Errorf("wrong modpack search: %s", req.URL)
			}
			json = `{"data":[{"id":10,"gameId":432,"classId":4471,"name":"Curse pack","slug":"curse-pack","isAvailable":true}],"pagination":{"totalCount":1}}`
		case "/v1/mods/10/files":
			json = fmt.Sprintf(`{"data":[{"id":20,"modId":10,"gameId":432,"displayName":"1.0","fileName":"pack.zip",
				"isAvailable":true,"gameVersions":["1.21.1","NeoForge"],"fileLength":%d,"hashes":[{"algo":1,"value":"%x"}],"downloadUrl":%q}]}`,
				len(archive), sha1.Sum(archive), archiveURL)
		case "/v1/mods":
			json = fmt.Sprintf(`{"data":[{"id":30,"gameId":432,"classId":6,"name":"Example","isAvailable":true,"allowModDistribution":%t}]}`, allow)
		case "/v1/mods/files":
			json = fmt.Sprintf(`{"data":[{"id":40,"modId":30,"gameId":432,"isAvailable":true,
				"fileName":"example.jar","fileLength":%d,"hashes":[{"algo":1,"value":"%x"}],"downloadUrl":%q}]}`, len(dependency), sha1.Sum([]byte(dependency)), uri)
		default:
			return nil, fmt.Errorf("unexpected API path: %s", req.URL.Path)
		}
		return fixtureResponse(http.StatusOK, []byte(json)), nil
	})}
	cf, ctx := curseForge{apiKey, api}, t.Context()
	found, err := cf.search(ctx, "Curse pack", "1.21.1", 0)
	if err != nil || len(found.Projects) != 1 {
		t.Fatalf("catalog = %+v, %v", found, err)
	}
	releases, _, err := cf.releases(ctx, found.Projects[0].ID, "1.21.1", 0)
	if err != nil || len(releases) != 1 || releases[0].Unavailable != "" {
		t.Fatalf("versions = %+v, %v", releases, err)
	}
	client := &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("x-api-key") != "" {
			return nil, errors.New("API key sent to CDN")
		}
		switch req.URL.String() {
		case uri:
			return packResponse([]byte(dependency)), nil
		case archiveURL:
			return packResponse(archive), nil
		default:
			return nil, fmt.Errorf("unexpected asset URL: %s", req.URL)
		}
	})}
	g := Game{Cache: t.TempDir()}
	downloaded, err := (distribution.Cache{Root: g.Cache, Client: client}).Ensure(ctx, releases[0].asset)
	if err != nil {
		t.Fatal(err)
	}
	online, err := modpack.Prepare(ctx, downloaded, CurseForgeModpacks)
	if err != nil {
		t.Fatal(err)
	}
	local, err := PrepareLocalModpack(ctx, packFile(t, "curse-pack.zip", entries))
	if err != nil || online.Info() != local.Info() || local.Info().Loader != LoaderNeoForge ||
		local.Info().LoaderVersion != "21.1.200" || local.Info().Downloads != 1 {
		t.Fatalf("online and local metadata disagree: %+v / %+v, %v", online.Info(), local.Info(), err)
	}
	store, _, next, i := packState(t, local.Info())
	verifyGame := func(_ context.Context, work Instance, _ func(Progress)) error {
		if !work.NeoForge() || work.LoaderVersion != "21.1.200" || work.GameDir == i.GameDir {
			return errors.New("incorrect staged NeoForge configuration")
		}
		return nil
	}
	if err := g.installModpack(ctx, store, next, i, local, cf, client, verifyGame, nil); err == nil || !strings.Contains(err.Error(), i18n.Text("curseforge.dependency_unavailable", "Example", 40)) {
		t.Fatalf("distribution restriction ignored: %v", err)
	}
	allow = true
	if err := g.installModpack(ctx, store, next, i, local, cf, client, verifyGame, nil); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(i.GameDir, "mods", "example.jar"))
	if err != nil || string(b) != dependency {
		t.Fatalf("CurseForge dependency missing: %q, %v", b, err)
	}
}

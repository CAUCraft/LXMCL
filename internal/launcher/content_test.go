package launcher

import (
	"archive/zip"
	"bytes"
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testZIP(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for name, value := range entries {
		f, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(value)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestModrinthDownloadsCompatibleRequiredDependencies(t *testing.T) {
	for _, tc := range []struct{ loader, version, marker string }{
		{LoaderFabric, "0.16.14", "fabric.mod.json"},
		{LoaderForge, "52.1.21", "META-INF/mods.toml"},
		{LoaderNeoForge, "21.1.265", "META-INF/neoforge.mods.toml"},
		{LoaderQuilt, "0.30.2", "quilt.mod.json"},
	} {
		t.Run(tc.loader, func(t *testing.T) {
			i := Instance{ID: 1, Version: "1.21.1", Loader: tc.loader, LoaderVersion: tc.version, GameDir: t.TempDir()}
			testModrinthDependencies(t, i, tc.marker)
		})
	}
}

func testModrinthDependencies(t *testing.T, i Instance, marker string) {
	t.Helper()
	ctx := t.Context()
	loader := strings.ToLower(i.Loader)
	body := testZIP(t, map[string]string{marker: "test metadata"})
	file := func(name string) PackFile {
		f := PackFile{URL: "https://cdn.modrinth.com/data/" + name, Filename: name, Size: int64(len(body)), Primary: true}
		f.Hashes.SHA512 = fmt.Sprintf("%x", sha512.Sum512(body))
		return f
	}
	core := PackVersion{ID: "versionCore", ProjectID: "projectCore", GameVersions: []string{i.Version}, Loaders: []string{loader}, Files: []PackFile{file("core.jar")}}
	dep := PackVersion{ID: "versionDep", ProjectID: "projectDep", Type: "release", GameVersions: []string{i.Version}, Loaders: []string{loader},
		Files: []PackFile{file("dep.jar")}, Dependencies: []ModDependency{{VersionID: core.ID, Type: "required"}}}
	mod := PackVersion{ID: "versionMod", ProjectID: "projectMod", GameVersions: []string{i.Version}, Loaders: []string{loader},
		Files: []PackFile{file("mod.jar")}, Dependencies: []ModDependency{{ProjectID: dep.ProjectID, Type: "required"}, {ProjectID: "ignored", Type: "optional"}}}
	wrongLoader, wrongGame := dep, dep
	wrongLoader.ID, wrongLoader.Loaders = "wrongLoader", []string{"other"}
	wrongGame.ID, wrongGame.GameVersions = "wrongGame", []string{"1.20.1"}
	encoded, err := json.Marshal([]PackVersion{wrongLoader, wrongGame, dep})
	if err != nil {
		t.Fatal(err)
	}
	encodeCore := func() []byte {
		b, err := json.Marshal(core)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	var downloadsSeen []string
	d := &downloads{http: &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/v2/project/"+dep.ProjectID+"/version" {
			q := req.URL.Query()
			if q.Get("loaders") != fmt.Sprintf(`[%q]`, loader) || q.Get("game_versions") != `["1.21.1"]` {
				t.Fatalf("dependencies used the wrong target filters: %s", req.URL)
			}
			return fixtureResponse(http.StatusOK, encoded), nil
		}
		if req.URL.String() == modrinthAPI+"version/"+core.ID {
			return fixtureResponse(http.StatusOK, encodeCore()), nil
		}
		if req.URL.Hostname() == "cdn.modrinth.com" {
			downloadsSeen = append(downloadsSeen, filepath.Base(req.URL.Path))
			return fixtureResponse(http.StatusOK, body), nil
		}
		t.Fatalf("unexpected dependency request: %s", req.URL)
		return nil, nil
	})}}
	valid := core.Loaders
	core.Loaders = []string{"other"}
	if err := downloadModrinth(ctx, i, Mods, mod, d, nil); err == nil || len(downloadsSeen) != 0 {
		t.Fatalf("incompatible pinned dependency did not block the download: %v, %v", err, downloadsSeen)
	}
	core.Loaders = valid
	var progress []Progress
	if err := downloadModrinth(ctx, i, Mods, mod, d, func(p Progress) { progress = append(progress, p) }); err != nil {
		t.Fatal(err)
	}
	if strings.Join(downloadsSeen, ",") != "core.jar,dep.jar,mod.jar" {
		t.Fatalf("dependency was not installed first: %v", downloadsSeen)
	}
	if last := progress[len(progress)-1]; last.Done != 3 || last.Total != 3 {
		t.Fatalf("verified mod download did not report all files: %+v", progress)
	}
	if err := downloadModrinth(ctx, i, Mods, mod, d, nil); err != nil || len(downloadsSeen) != 3 {
		t.Fatalf("verified local files were not reused: %v, %v", downloadsSeen, err)
	}
}

func TestQuiltChoosesOneFabricAPIProvider(t *testing.T) {
	for _, tc := range []struct{ game, want string }{{"1.20.1", quiltedAPIProject}, {"1.21.1", fabricAPIProject}} {
		t.Run(tc.game, func(t *testing.T) {
			i := Instance{Version: tc.game, Loader: LoaderQuilt}
			main := PackVersion{ID: "mainVersion", ProjectID: "mainProject", GameVersions: []string{i.Version}, Loaders: []string{"quilt"},
				Dependencies: []ModDependency{{ProjectID: fabricAPIProject, Type: "required"}}}
			if tc.want == quiltedAPIProject {
				main.Dependencies = append(main.Dependencies, ModDependency{ProjectID: quiltedAPIProject, Type: "required"})
			}
			var calls int
			d := &downloads{http: &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				project := strings.Split(req.URL.Path, "/")[3]
				loader := req.URL.Query().Get("loaders")
				if req.URL.Query().Get("game_versions") != fmt.Sprintf(`[%q]`, i.Version) {
					t.Fatalf("API dependency used a different Minecraft version: %s", req.URL)
				}
				if project == quiltedAPIProject && tc.want == quiltedAPIProject && loader == `["quilt"]` {
					return fixtureResponse(200, []byte(fmt.Sprintf(`[{"id":"api","project_id":%q,"game_versions":[%q],"loaders":["quilt"],"files":[{"filename":"qfapi.jar"}]}]`, project, i.Version))), nil
				}
				if project == fabricAPIProject && tc.want == fabricAPIProject && loader == `["fabric"]` {
					return fixtureResponse(200, []byte(fmt.Sprintf(`[{"id":"api","project_id":%q,"game_versions":[%q],"loaders":["fabric"],"files":[{"filename":"fapi.jar"}]}]`, project, i.Version))), nil
				}
				return fixtureResponse(200, []byte(`[]`)), nil
			})}}
			plan, err := modPlan(t.Context(), i, main, d)
			wantCalls := 2
			if tc.want == fabricAPIProject {
				// Only Fabric API was requested on this version. QFAPI is unavailable.
				wantCalls = 3
			}
			if err != nil || len(plan) != 2 || plan[0].ProjectID != tc.want || calls != wantCalls {
				t.Fatalf("expected one compatible Fabric API provider: %v, %v (%d requests)", plan, err, calls)
			}
		})
	}
}

func TestModrinthPackVerifiesContentAndTargetsInstance(t *testing.T) {
	content := testZIP(t, map[string]string{"pack.mcmeta": "{}"})
	instance := Instance{ID: 2, Version: "1.21.1", GameDir: t.TempDir()}
	other := Instance{ID: 1, Version: "1.21.1", GameDir: t.TempDir()}
	d := &downloads{http: &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Hostname() != "cdn.modrinth.com" {
			t.Fatalf("unexpected origin: %s", req.URL)
		}
		return fixtureResponse(http.StatusOK, content), nil
	})}}
	file := PackFile{URL: "https://cdn.modrinth.com/data/pack.zip", Filename: "pack.zip", Size: int64(len(content)), Primary: true}
	file.Hashes.SHA512 = strings.Repeat("0", 128)
	version := PackVersion{GameVersions: []string{"1.21.1"}, Files: []PackFile{file}}
	if err := downloadModrinth(t.Context(), instance, ResourcePacks, version, d, nil); err == nil {
		t.Fatal("accepted a pack with a mismatched SHA-512")
	}
	if files, err := ListContent(instance, ResourcePacks); len(files) != 0 || err != nil {
		t.Fatalf("bad download left files: %v, %v", files, err)
	}
	version.Files[0].Hashes.SHA512 = fmt.Sprintf("%x", sha512.Sum512(content))
	if err := downloadModrinth(t.Context(), instance, ResourcePacks, version, d, nil); err != nil {
		t.Fatal(err)
	}
	if err := SetContentEnabled(instance, ResourcePacks, "pack.zip", false); err != nil {
		t.Fatal(err)
	}
	if files, err := ListContent(instance, ResourcePacks); err != nil || len(files) != 1 || files[0].Enabled {
		t.Fatalf("disabled pack not listed: %v, %v", files, err)
	}
	if files, err := ListContent(other, ResourcePacks); len(files) != 0 || err != nil {
		t.Fatalf("changed the wrong instance: %v, %v", files, err)
	}
}

func TestModrinthSearchPaginatesAndUsesCorrectCategory(t *testing.T) {
	var got []string
	d := &downloads{http: &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		q := req.URL.Query()
		if req.URL.Path != "/v2/search" || q.Get("query") != "test" || q.Get("index") != "updated" ||
			q.Get("limit") != "20" || q.Get("offset") != "40" || !strings.Contains(q.Get("facets"), "versions:1.21.1") {
			t.Fatalf("unexpected search request: %s", req.URL)
		}
		got = append(got, q.Get("facets"))
		return fixtureResponse(200, []byte(`{"total_hits":54,"hits":[{"project_id":"valid","title":"Demo","description":"Pack"},{"project_id":"../bad","title":"Invalid"}]}`)), nil
	})}}
	for _, tc := range []struct {
		kind          ContentKind
		loader, facet string
	}{
		{Mods, LoaderFabric, "categories:fabric"},
		{Mods, LoaderForge, "categories:forge"},
		{Mods, LoaderNeoForge, "categories:neoforge"},
		{Mods, LoaderQuilt, "categories:quilt"},
		{ShaderPacks, LoaderFabric, "project_type:shader"},
	} {
		result, err := searchModrinth(t.Context(), Instance{Version: "1.21.1", Loader: tc.loader}, tc.kind, " test ", "updated", 2, d)
		if err != nil || result.Total != 54 || len(result.Hits) != 1 || result.Hits[0].Description != "Pack" {
			t.Fatalf("unexpected result: %+v / %v", result, err)
		}
		facets := got[len(got)-1]
		if !strings.Contains(facets, tc.facet) || tc.kind == ShaderPacks && strings.Contains(facets, "categories:") {
			t.Fatalf("incompatible search facets: %s", facets)
		}
	}
}

func TestShaderPackRequiresShadersDirectoryAndFabricInstance(t *testing.T) {
	i := Instance{GameDir: t.TempDir(), Loader: "Vanilla"}
	source := filepath.Join(t.TempDir(), "test.zip")
	if err := os.WriteFile(source, testZIP(t, map[string]string{"shaders/composite.fsh": "// sample"}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ImportContent(t.Context(), i, ShaderPacks, source); err == nil {
		t.Fatal("offered an unconfigured vanilla instance as a shader target")
	}
	i.Loader = "Fabric"
	if err := ImportContent(t.Context(), i, ShaderPacks, source); err != nil {
		t.Fatal(err)
	}
	if files, err := ListContent(i, ShaderPacks); err != nil || len(files) != 1 || files[0].Name != "test.zip" {
		t.Fatalf("shader not installed: %v %v", files, err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(source), "bad.zip"), testZIP(t, map[string]string{"pack.mcmeta": "{}"}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ImportContent(t.Context(), i, ShaderPacks, filepath.Join(filepath.Dir(source), "bad.zip")); err == nil {
		t.Fatal("accepted a resource pack as a shader")
	}
}

func TestImportFabricModDoesNotReplaceExistingFile(t *testing.T) {
	content := testZIP(t, map[string]string{"fabric.mod.json": "{}"})
	source := filepath.Join(t.TempDir(), "example.jar")
	if err := os.WriteFile(source, content, 0600); err != nil {
		t.Fatal(err)
	}
	i := Instance{GameDir: t.TempDir()}
	if err := ImportContent(t.Context(), i, Mods, source); err == nil {
		t.Fatal("imported a Fabric mod into a vanilla instance")
	}
	i.Loader = "Fabric"
	if err := ImportContent(t.Context(), i, Mods, source); err != nil {
		t.Fatal(err)
	}
	if err := ImportContent(t.Context(), i, Mods, source); err == nil {
		t.Fatal("overwrote the existing mod")
	}
}

func TestBackupAndRestoreKeepExistingWorld(t *testing.T) {
	i := Instance{ID: 3, GameDir: t.TempDir()}
	dir := filepath.Join(i.GameDir, "saves", "Survival")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	level := filepath.Join(dir, "level.dat")
	if err := os.WriteFile(level, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	backup, err := BackupSaves(t.Context(), i, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(level, []byte("newer"), 0600); err != nil {
		t.Fatal(err)
	}
	if n, err := RestoreSaves(t.Context(), i, backup); err != nil || n != 1 {
		t.Fatalf("restore returned %d, %v", n, err)
	}
	got, _ := os.ReadFile(level)
	copy, _ := os.ReadFile(filepath.Join(filepath.Dir(dir), "Survival (2)", "level.dat"))
	if string(got) != "newer" || string(copy) != "original" {
		t.Fatalf("restore replaced the current world or lost the backup: %q %q", got, copy)
	}
	malicious := filepath.Join(t.TempDir(), "unsafe.zip")
	if err := os.WriteFile(malicious, testZIP(t, map[string]string{"../outside/level.dat": "evil", "World/level.dat": "valid"}), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := RestoreSaves(t.Context(), i, malicious); err == nil {
		t.Fatal("accepted a world archive with path traversal")
	}
}

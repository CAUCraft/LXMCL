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

func TestModrinthDownloadsRequiredFabricDependency(t *testing.T) {
	ctx := t.Context()
	i := Instance{ID: 1, Version: "1.21.1", Loader: "Fabric", LoaderVersion: "0.16.14", GameDir: t.TempDir()}
	body := testZIP(t, map[string]string{"fabric.mod.json": "{}"})
	file := func(name string) PackFile {
		f := PackFile{URL: "https://cdn.modrinth.com/data/" + name, Filename: name, Size: int64(len(body)), Primary: true}
		f.Hashes.SHA512 = fmt.Sprintf("%x", sha512.Sum512(body))
		return f
	}
	dep := PackVersion{ID: "versionDep", ProjectID: "projectDep", GameVersions: []string{i.Version}, Loaders: []string{"fabric"}, Files: []PackFile{file("dep.jar")}}
	mod := PackVersion{ID: "versionMod", ProjectID: "projectMod", GameVersions: []string{i.Version}, Loaders: []string{"fabric"},
		Files: []PackFile{file("mod.jar")}, Dependencies: []ModDependency{{ProjectID: dep.ProjectID, VersionID: dep.ID, Type: "required"}}}
	encoded, err := json.Marshal(dep)
	if err != nil {
		t.Fatal(err)
	}
	var downloadsSeen []string
	d := &downloads{http: &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() == modrinthAPI+"version/"+dep.ID {
			return fixtureResponse(http.StatusOK, encoded), nil
		}
		if req.URL.Hostname() == "cdn.modrinth.com" {
			downloadsSeen = append(downloadsSeen, filepath.Base(req.URL.Path))
			return fixtureResponse(http.StatusOK, body), nil
		}
		t.Fatalf("unexpected dependency request: %s", req.URL)
		return nil, nil
	})}}
	var progress []Progress
	if err := downloadModrinth(ctx, i, FabricMods, mod, d, func(p Progress) { progress = append(progress, p) }); err != nil {
		t.Fatal(err)
	}
	if strings.Join(downloadsSeen, ",") != "dep.jar,mod.jar" {
		t.Fatalf("dependency was not installed first: %v", downloadsSeen)
	}
	if last := progress[len(progress)-1]; last.Done != 2 || last.Total != 2 {
		t.Fatalf("verified mod download did not report both files: %+v", progress)
	}
	if err := downloadModrinth(ctx, i, FabricMods, mod, d, nil); err != nil || len(downloadsSeen) != 2 {
		t.Fatalf("verified local files were not reused: %v, %v", downloadsSeen, err)
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
	for _, kind := range []ContentKind{FabricMods, ShaderPacks} {
		result, err := searchModrinth(t.Context(), "1.21.1", kind, " test ", "updated", 2, d)
		if err != nil || result.Total != 54 || len(result.Hits) != 1 || result.Hits[0].Description != "Pack" {
			t.Fatalf("unexpected result: %+v / %v", result, err)
		}
	}
	if !strings.Contains(got[0], "categories:fabric") || !strings.Contains(got[1], "project_type:shader") ||
		strings.Contains(got[1], "categories:fabric") {
		t.Fatalf("incompatible search facets: %v", got)
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
	if err := ImportContent(t.Context(), i, FabricMods, source); err == nil {
		t.Fatal("imported a Fabric mod into a vanilla instance")
	}
	i.Loader = "Fabric"
	if err := ImportContent(t.Context(), i, FabricMods, source); err != nil {
		t.Fatal(err)
	}
	if err := ImportContent(t.Context(), i, FabricMods, source); err == nil {
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

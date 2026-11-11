package launcher

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha512"
	"errors"
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

func TestModrinthDownloadOnlyInstallsSelectedFile(t *testing.T) {
	for _, tc := range []struct{ loader, version, marker string }{
		{LoaderFabric, "0.16.14", "fabric.mod.json"},
		{LoaderForge, "52.1.21", "META-INF/mods.toml"},
		{LoaderNeoForge, "21.1.265", "META-INF/neoforge.mods.toml"},
		{LoaderQuilt, "0.30.2", "quilt.mod.json"},
	} {
		t.Run(tc.loader, func(t *testing.T) {
			i := Instance{ID: 1, Version: "1.21.1", Loader: tc.loader, LoaderVersion: tc.version, GameDir: t.TempDir()}
			body := testZIP(t, map[string]string{tc.marker: "test metadata"})
			const fileURL = "https://cdn.modrinth.com/data/selected.jar"
			file := PackFile{URL: fileURL, Filename: "selected.jar", Size: int64(len(body)), Primary: true}
			file.Hashes.SHA512 = fmt.Sprintf("%x", sha512.Sum512(body))
			v := PackVersion{ID: "selectedVersion", ProjectID: "selectedProject", GameVersions: []string{i.Version},
				Loaders: []string{strings.ToLower(i.Loader)}, Files: []PackFile{file},
				Dependencies: []ModDependency{{ProjectID: "requiredProject", VersionID: "missingVersion", Type: "required"}}}
			var calls int
			g := Game{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.String() != fileURL {
					t.Fatalf("install requested dependency metadata or files: %s", req.URL)
				}
				calls++
				return fixtureResponse(http.StatusOK, body), nil
			})}
			var progress Progress
			if err := g.DownloadModrinth(t.Context(), i, Mods, v, func(p Progress) { progress = p }); err != nil {
				t.Fatal(err)
			}
			files, err := ListContent(i, Mods)
			if err != nil || len(files) != 1 || files[0].Name != file.Filename || progress.Done != 1 || progress.Total != 1 {
				t.Fatalf("selected file was not installed alone: %v %v %+v", files, err, progress)
			}
			if err := g.DownloadModrinth(t.Context(), i, Mods, v, nil); err != nil || calls != 1 {
				t.Fatalf("retry did not reuse the verified file: %d requests, %v", calls, err)
			}
		})
	}
}

func TestExistingResourceMustMatchLoaderFormat(t *testing.T) {
	i := Instance{ID: 1, Version: "1.21.1", Loader: LoaderFabric, LoaderVersion: "0.16.14", GameDir: t.TempDir()}
	// An exact hash match alone must not turn a Forge archive into a valid Fabric mod.
	body := testZIP(t, map[string]string{"META-INF/mods.toml": "forge metadata"})
	file := PackFile{URL: "https://cdn.modrinth.com/wrong.jar", Filename: "wrong.jar", Size: int64(len(body))}
	file.Hashes.SHA512 = fmt.Sprintf("%x", sha512.Sum512(body))
	v := PackVersion{ID: "version", ProjectID: "project", GameVersions: []string{i.Version}, Loaders: []string{"fabric"}, Files: []PackFile{file}}
	if err := os.MkdirAll(filepath.Join(i.GameDir, "mods"), 0755); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(i.GameDir, "mods", file.Filename)
	if err := os.WriteFile(filename, body, 0600); err != nil {
		t.Fatal(err)
	}
	g := Game{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("existing invalid resource triggered a network request")
		return nil, nil
	})}
	if err := g.DownloadModrinth(t.Context(), i, Mods, v, nil); err == nil {
		t.Fatal("existing resource bypassed archive format validation")
	}
	if got, err := os.ReadFile(filename); err != nil || !bytes.Equal(got, body) {
		t.Fatalf("existing file was modified: %v", err)
	}
}

func TestModrinthPackVerifiesContentAndTargetsInstance(t *testing.T) {
	content := testZIP(t, map[string]string{"pack.mcmeta": "{}"})
	instance := Instance{ID: 2, Version: "1.21.1", GameDir: t.TempDir()}
	other := Instance{ID: 1, Version: "1.21.1", GameDir: t.TempDir()}
	g := Game{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Hostname() != "cdn.modrinth.com" {
			t.Fatalf("unexpected origin: %s", req.URL)
		}
		return fixtureResponse(http.StatusOK, content), nil
	})}
	file := PackFile{URL: "https://cdn.modrinth.com/data/pack.zip", Filename: "pack.zip", Size: int64(len(content)), Primary: true}
	file.Hashes.SHA512 = strings.Repeat("0", 128)
	version := PackVersion{GameVersions: []string{"1.21.1"}, Files: []PackFile{file}}
	if err := g.DownloadModrinth(t.Context(), instance, ResourcePacks, version, nil); err == nil {
		t.Fatal("accepted a pack with a mismatched SHA-512")
	}
	if files, err := ListContent(instance, ResourcePacks); len(files) != 0 || err != nil {
		t.Fatalf("bad download left files: %v, %v", files, err)
	}
	version.Files[0].Hashes.SHA512 = fmt.Sprintf("%x", sha512.Sum512(content))
	if err := g.DownloadModrinth(t.Context(), instance, ResourcePacks, version, nil); err != nil {
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
	conflict := testZIP(t, map[string]string{
		"World/level.dat": "valid", "World/data": "file", "World/data/chunk": "nested",
	})
	if err := os.WriteFile(malicious, conflict, 0600); err != nil {
		t.Fatal(err)
	}
	i.GameDir = filepath.Join(t.TempDir(), "uncreated")
	if n, err := RestoreSaves(t.Context(), i, malicious); err == nil || n != 0 {
		t.Fatalf("accepted conflicting archive paths: %d, %v", n, err)
	}
	if _, err := os.Stat(i.GameDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid restore created its destination: %v", err)
	}
}

func TestCanceledSaveOperationsDoNotCreateDirectories(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	root := t.TempDir()
	i := Instance{ID: 3, GameDir: filepath.Join(root, "game")}
	if name, err := BackupSaves(ctx, i, filepath.Join(root, "data")); name != "" || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled backup returned %q, %v", name, err)
	}
	if n, err := RestoreSaves(ctx, i, filepath.Join(root, "absent.zip")); n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled restore returned %d, %v", n, err)
	}
	if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
		t.Fatalf("canceled operations created files: %v, %v", entries, err)
	}
}

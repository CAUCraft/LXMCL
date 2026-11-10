package launcher

import (
	"bytes"
	"crypto/sha512"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestResourceFolderDownloadUsesSelectedDirectory(t *testing.T) {
	g := Game{Cache: t.TempDir()}
	folder := t.TempDir()
	body := testZIP(t, map[string]string{"fabric.mod.json": "{}"})
	f := PackFile{URL: "https://cdn.modrinth.com/data/test/sample.jar", Filename: "sample.jar", Size: int64(len(body))}
	f.Hashes.SHA512 = fmt.Sprintf("%x", sha512.Sum512(body))
	cache := filepath.Join(g.Cache, "modpacks", "sha512")
	if err := os.MkdirAll(cache, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, f.Hashes.SHA512), body, 0600); err != nil {
		t.Fatal(err)
	}
	v := PackVersion{Files: []PackFile{f}, Dependencies: []ModDependency{{ProjectID: "not-requested", Type: "required"}}}
	if err := g.DownloadResourceFile(t.Context(), Mods, v, folder, nil); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(folder, f.Filename)
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, body) {
		t.Fatalf("verified resource was not saved directly in the selected folder: %v", err)
	}
	const existing = "keep this file"
	if err := os.WriteFile(path, []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}
	if err := g.DownloadResourceFile(t.Context(), Mods, v, folder, nil); err == nil {
		t.Fatal("a same-name file was not reported")
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != existing {
		t.Fatal("the existing file was changed")
	}
}

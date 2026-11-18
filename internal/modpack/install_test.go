package modpack_test

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/distribution"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/modpack"
	"github.com/CAUCraft/LXMCL/internal/tasks"
)

type resolver func(context.Context, []modpack.Reference) ([]modpack.Download, error)

func (f resolver) Resolve(ctx context.Context, refs []modpack.Reference) ([]modpack.Download, error) {
	return f(ctx, refs)
}

func archive(t *testing.T, manifest string) string {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "pack.zip")
	f, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	w, err := z.Create("manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(manifest)); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return filename
}

const requiredPack = `{"manifestType":"minecraftModpack","manifestVersion":1,"minecraft":{"version":"1.21.1"},"files":[{"projectID":1,"fileID":2}]}`

func options(t *testing.T) modpack.InstallOptions {
	t.Helper()
	return modpack.InstallOptions{
		Target: instance.Instance{ID: 1, Revision: 1, Name: "Pack", Version: "1.21.1", Loader: instance.LoaderVanilla, GameDir: filepath.Join(t.TempDir(), "instance")},
		Cache:  distribution.Cache{Root: t.TempDir()},
		InstallGame: func(context.Context, instance.Instance, func(tasks.Progress)) error {
			t.Error("invalid plan reached game installation")
			return nil
		},
		Commit: func() error { t.Error("invalid plan committed instance state"); return nil },
	}
}

func TestInstallRejectsMissingOrInvalidResolvedFiles(t *testing.T) {
	p, err := modpack.PrepareLocal(t.Context(), archive(t, requiredPack))
	if err != nil {
		t.Fatal(err)
	}
	a := distribution.Asset{URLs: []string{"https://edge.forgecdn.net/file"}, Algorithm: "sha512", Digest: fmt.Sprintf("%x", sha512.Sum512(nil))}
	for _, tc := range []struct {
		name    string
		resolve modpack.Resolver
	}{
		{"missing resolver", nil},
		{"missing file", resolver(func(context.Context, []modpack.Reference) ([]modpack.Download, error) { return nil, nil })},
		{"invalid metadata", resolver(func(context.Context, []modpack.Reference) ([]modpack.Download, error) {
			return []modpack.Download{{Path: "mods/test.jar"}}, nil
		})},
		{"escaping path", resolver(func(context.Context, []modpack.Reference) ([]modpack.Download, error) {
			return []modpack.Download{{Path: "../outside", Asset: a}}, nil
		})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := options(t)
			o.Resolver = tc.resolve
			if err := modpack.Install(t.Context(), p, o, nil); err == nil {
				t.Fatal("invalid resolved plan installed")
			}
			entries, err := os.ReadDir(filepath.Dir(o.Target.GameDir))
			if err != nil || len(entries) != 0 {
				t.Fatalf("invalid plan left files: %v, %v", entries, err)
			}
		})
	}
}

func TestPreviewFingerprintAndRequiredServices(t *testing.T) {
	filename := archive(t, requiredPack)
	p, err := modpack.PrepareLocal(t.Context(), filename)
	if err != nil {
		t.Fatal(err)
	}
	other, err := modpack.PrepareLocal(t.Context(), filename)
	if err != nil || p != other {
		t.Fatalf("preview lost comparable identity: %v", err)
	}
	o := options(t)
	missing := o
	missing.Commit = nil
	if err := modpack.Install(t.Context(), p, missing, nil); err == nil {
		t.Fatal("missing commit accepted")
	}
	missing = o
	missing.InstallGame = nil
	if err := modpack.Install(t.Context(), p, missing, nil); err == nil {
		t.Fatal("missing game installer accepted")
	}
	b, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	b[len(b)-1] ^= 1
	if err := os.WriteFile(filename, b, 0600); err != nil {
		t.Fatal(err)
	}
	o.Resolver = resolver(func(context.Context, []modpack.Reference) ([]modpack.Download, error) {
		t.Error("changed archive reached resolver")
		return nil, nil
	})
	if err := modpack.Install(t.Context(), p, o, nil); err == nil {
		t.Fatal("changed archive installed")
	}
}

func TestInstallPreservesDestinationCreatedBeforePublication(t *testing.T) {
	filename := archive(t, `{"manifestType":"minecraftModpack","manifestVersion":1,"minecraft":{"version":"1.21.1"}}`)
	p, err := modpack.PrepareLocal(t.Context(), filename)
	if err != nil {
		t.Fatal(err)
	}
	o := options(t)
	o.InstallGame = func(_ context.Context, work instance.Instance, _ func(tasks.Progress)) error {
		return os.WriteFile(filepath.Join(work.GameDir, "installed.txt"), []byte("verified game"), 0600)
	}
	err = modpack.Install(t.Context(), p, o, func(p tasks.Progress) {
		if p.Stage == i18n.Text("progress.save_instance") {
			if err := os.Mkdir(o.Target.GameDir, 0755); err != nil {
				t.Fatal(err)
			}
		}
	})
	if err == nil {
		t.Fatal("installation replaced a concurrently created directory")
	}
	entries, err := os.ReadDir(o.Target.GameDir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("existing directory was changed: %v, %v", entries, err)
	}
	entries, err = os.ReadDir(filepath.Dir(o.Target.GameDir))
	if err != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(o.Target.GameDir) {
		t.Fatalf("staging directory was not cleaned: %v, %v", entries, err)
	}
}

func TestPortableSelectionRoundTrip(t *testing.T) {
	ctx := t.Context()
	o := options(t)
	i := o.Target
	i.GameDir = t.TempDir()
	i.Name = "Source"
	for _, name := range []string{"mods/local.jar.disabled", "saves/world/level.dat", "config/local.txt"} {
		filename := filepath.Join(i.GameDir, name)
		if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
	}
	files := modpack.Selection{Mods: true, Config: true}
	filename, err := modpack.Export(ctx, i, t.TempDir(), "1.0", files)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := modpack.PrepareLocal(ctx, filename)
	if err != nil || preview.Info().Overrides != 2 {
		t.Fatalf("preview: %+v %v", preview.Info(), err)
	}
	o.Commit = func() error { return nil }
	o.InstallGame = func(context.Context, instance.Instance, func(tasks.Progress)) error { return nil }
	if err := modpack.Install(ctx, preview, o, nil); err != nil {
		t.Fatal(err)
	}
	dup := o.Target
	dup.GameDir = filepath.Join(t.TempDir(), "copy")
	if err := modpack.Duplicate(ctx, i, dup, files, o.Commit); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{dup.GameDir, o.Target.GameDir} {
		for _, name := range []string{"mods/local.jar.disabled", "config/local.txt"} {
			b, err := os.ReadFile(filepath.Join(dir, name))
			if string(b) != name || err != nil {
				t.Fatalf("copied %s: %v", name, err)
			}
		}
		if _, err := os.Stat(filepath.Join(dir, "saves")); !os.IsNotExist(err) {
			t.Fatalf("unexpected saves %v", err)
		}
	}
}

func TestUpdatePreservesPlayerFilesAndRollsBackFailedCommit(t *testing.T) {
	ctx := t.Context()
	o := options(t)
	source := o.Target
	source.GameDir = t.TempDir()
	out := t.TempDir()
	put := func(dir, name, value string) {
		t.Helper()
		f := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(f), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte(value), 0644); err != nil {
			t.Fatal(err)
		}
	}
	pack := func(version string) modpack.Prepared {
		t.Helper()
		f, err := modpack.Export(ctx, source, out, version, modpack.Selection{Mods: true, Config: true, Worlds: true})
		if err != nil {
			t.Fatal(err)
		}
		p, err := modpack.PrepareLocal(ctx, f)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	for _, name := range []string{"mods/a.jar", "mods/obsolete.jar", "config/client", "saves/world/level.dat"} {
		put(source.GameDir, name, "initial")
	}
	o.Commit = func() error { return nil }
	o.InstallGame = func(context.Context, instance.Instance, func(tasks.Progress)) error { return nil }
	if err := modpack.Install(ctx, pack("1"), o, nil); err != nil {
		t.Fatal(err)
	}
	put(o.Target.GameDir, "config/client", "player")
	put(o.Target.GameDir, "saves/world/level.dat", "played")
	for _, name := range []string{"mods/a.jar", "mods/new.jar", "config/client", "saves/world/level.dat"} {
		put(source.GameDir, name, "upstream")
	}
	if err := os.Remove(filepath.Join(source.GameDir, "mods/obsolete.jar")); err != nil {
		t.Fatal(err)
	}
	latest := pack("2")
	preview, err := modpack.PreviewUpdate(ctx, latest, o)
	if err != nil || !preview.Tracked || len(preview.Changes) != 4 {
		t.Fatalf("preview %+v: %v", preview, err)
	}
	o.Commit = func() error { return errors.New("disk full") }
	check := func(name, want string) {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(o.Target.GameDir, name))
		if err != nil || string(b) != want {
			t.Fatalf("%s: got %q, %v", name, b, err)
		}
	}
	if err := modpack.ApplyUpdate(ctx, preview, o, nil); err == nil {
		t.Fatal("accepted failed commit")
	}
	check("mods/a.jar", "initial")
	check("mods/obsolete.jar", "initial")
	if _, err := os.Stat(filepath.Join(o.Target.GameDir, "mods/new.jar")); !os.IsNotExist(err) {
		t.Fatalf("new file after rollback: %v", err)
	}
	o.Commit = func() error { return nil }
	put(o.Target.GameDir, "mods/a.jar", "changed after preview")
	if err := modpack.ApplyUpdate(ctx, preview, o, nil); err == nil {
		t.Fatal("accepted changed preview")
	}
	put(o.Target.GameDir, "mods/a.jar", "initial")
	if err := modpack.ApplyUpdate(ctx, preview, o, nil); err != nil {
		t.Fatal(err)
	}
	check("mods/a.jar", "upstream")
	check("mods/new.jar", "upstream")
	check("config/client", "player")
	check("saves/world/level.dat", "played")
	if _, err := os.Stat(filepath.Join(o.Target.GameDir, "mods/obsolete.jar")); !os.IsNotExist(err) {
		t.Fatalf("obsolete mod: %v", err)
	}
}

func TestServerFeedVerifiesAndReusesPublishedPack(t *testing.T) {
	ctx := t.Context()
	i := options(t).Target
	i.GameDir = t.TempDir()
	file, err := modpack.Export(ctx, i, t.TempDir(), "2.0", modpack.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(body))
	var downloads atomic.Int32
	var corrupt atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pack.json":
			json.NewEncoder(w).Encode(map[string]any{"file": "client.mrpack", "size": len(body), "sha256": digest})
		case "/client.mrpack":
			downloads.Add(1)
			if corrupt.Load() {
				w.Write([]byte("incomplete"))
				return
			}
			w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	feed := server.URL + "/pack.json"
	cache := t.TempDir()
	read := func(root, previous string) (modpack.Prepared, bool, error) {
		return modpack.PrepareFeed(ctx, feed, root, previous, server.Client().Transport)
	}
	p, same, err := read(cache, "")
	if err != nil || same || p.Info().Version != "2.0" {
		t.Fatalf("initial subscription: %v %v", p.Info(), err)
	}
	if u, sum := p.Feed(); u != feed || sum != digest {
		t.Fatalf("source mismatch %s %s", u, sum)
	}
	if _, same, err := read(cache, digest); err != nil || !same || downloads.Load() != 1 {
		t.Fatalf("unchanged release: %v %v", same, err)
	}
	if _, _, err := read(cache, ""); err != nil || downloads.Load() != 1 {
		t.Fatalf("verified cache: %v", err)
	}
	corrupt.Store(true)
	if _, _, err := read(t.TempDir(), ""); err == nil {
		t.Fatal("accepted truncated server archive")
	}
}

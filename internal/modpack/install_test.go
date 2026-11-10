package modpack_test

import (
	"archive/zip"
	"context"
	"crypto/sha512"
	"fmt"
	"os"
	"path/filepath"
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

package log

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestExportOriginalLog(t *testing.T) {
	game, out := t.TempDir(), t.TempDir()
	var archive bytes.Buffer
	z := zip.NewWriter(&archive)
	member, err := z.Create("game.log")
	if err != nil {
		t.Fatal(err)
	}
	raw := bytes.Repeat([]byte{0xff, 0, 'a', '\r', '\n'}, tailLimit/4)
	if _, err := member.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	write := func(name string, data []byte) string {
		t.Helper()
		path := filepath.Join(game, "logs", name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	latest := write("latest.log", raw)
	zipped := write("2026-11-26-1.log.zip", archive.Bytes())
	write("unrelated.zip", archive.Bytes())
	entries, err := Recent(t.Context(), game)
	if err != nil || len(entries) != 2 {
		t.Fatal(entries, err)
	}
	byPath := make(map[string]Entry, len(entries))
	for _, e := range entries {
		byPath[e.Path] = e
	}
	if !IsInstanceLog(zipped) || IsPreview(zipped) {
		t.Fatal("ZIP log archive should be selectable without preview decoding")
	}
	if err := ExportPreview(latest, zipped, "preview"); err == nil {
		t.Fatal("preview export could replace a cataloged ZIP log")
	}
	// New bytes written while the chooser was open belong to the live log copy.
	live := append(bytes.Clone(raw), []byte("appended")...)
	write("latest.log", live)
	assertBytes := func(path string, want []byte) {
		t.Helper()
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("%s: %d bytes, want %d, error %v", filepath.Base(path), len(got), len(want), err)
		}
	}
	for source, want := range map[string][]byte{latest: live, zipped: archive.Bytes()} {
		dest := filepath.Join(out, filepath.Base(source))
		entry := byPath[source]
		if err := entry.ExportOriginal(t.Context(), game, dest); err != nil {
			t.Fatal(err)
		}
		if err := entry.ExportOriginal(t.Context(), game, dest); !errors.Is(err, fs.ErrExist) {
			t.Fatal("existing target not rejected", err)
		}
		assertBytes(dest, want)
		assertBytes(source, want)
	}

	stale := byPath[zipped]
	noCopy := filepath.Join(out, "stale.zip")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := stale.ExportOriginal(ctx, game, noCopy); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled export not rejected", err)
	}
	if err := stale.ExportOriginal(t.Context(), t.TempDir(), noCopy); !errors.Is(err, ErrOriginalChanged) {
		t.Fatal("wrong instance not rejected", err)
	}
	if err := stale.ExportOriginal(t.Context(), game, filepath.Join(out, "wrong.txt")); err == nil {
		t.Fatal("export did not preserve the archive extension")
	}
	if err := os.Rename(zipped, zipped+".moved"); err != nil {
		t.Fatal(err)
	}
	if err := stale.ExportOriginal(t.Context(), game, noCopy); !errors.Is(err, ErrOriginalChanged) {
		t.Fatal("missing source not rejected", err)
	}
	write(filepath.Base(zipped), archive.Bytes())
	if err := os.Chtimes(zipped, stale.Modified, stale.Modified); err != nil {
		t.Fatal(err)
	}
	if err := stale.ExportOriginal(t.Context(), game, noCopy); !errors.Is(err, ErrOriginalChanged) {
		t.Fatal("same-name replacement not rejected", err)
	}
	if saved, err := os.ReadDir(out); err != nil || len(saved) != 2 {
		t.Fatal("failure left an extra file", saved, err)
	}
}

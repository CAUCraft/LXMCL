package content_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha512"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

func resourceArchive(t *testing.T, marker string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.Create(marker)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("{}")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type observedReader struct {
	source  io.Reader
	observe func()
}

func (r observedReader) Read(p []byte) (int, error) {
	if len(p) > 32 {
		p = p[:32]
	}
	n, err := r.source.Read(p)
	r.observe()
	return n, err
}

func TestWriteVerifiedHidesResourceUntilValidation(t *testing.T) {
	i := instance.Instance{ID: 7, Version: "1.21.1", GameDir: t.TempDir()}
	body := resourceArchive(t, "pack.mcmeta")
	name := filepath.Join(i.GameDir, "resourcepacks", "pack.zip")
	reader := observedReader{bytes.NewReader(body), func() {
		if _, err := os.Lstat(name); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("unverified resource visible: %v", err)
		}
		files, err := content.List(i, content.ResourcePacks)
		if err != nil || len(files) != 0 {
			t.Fatalf("staging file listed as a resource: %v, %v", files, err)
		}
	}}
	if err := content.WriteVerified(t.Context(), i, content.ResourcePacks, "pack.zip", reader, int64(len(body)), sha512.New(), fmt.Sprintf("%x", sha512.Sum512(body))); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(name)
	if err != nil || !bytes.Equal(data, body) {
		t.Fatalf("published resource differs: %v", err)
	}
	files, err := content.List(i, content.ResourcePacks)
	if err != nil || len(files) != 1 || files[0].Name != "pack.zip" || !files[0].Enabled {
		t.Fatalf("published resource missing: %v, %v", files, err)
	}
}

func TestWriteVerifiedRejectsInvalidOrCanceledResources(t *testing.T) {
	for _, mode := range []string{"digest", "truncated", "archive marker", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			i := instance.Instance{ID: 7, GameDir: t.TempDir()}
			marker := "pack.mcmeta"
			if mode == "archive marker" {
				marker = "wrong.json"
			}
			body := resourceArchive(t, marker)
			size := int64(len(body))
			digest := fmt.Sprintf("%x", sha512.Sum512(body))
			if mode == "digest" {
				digest = strings.Repeat("0", 128)
			}
			if mode == "truncated" {
				size++
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			reader := observedReader{bytes.NewReader(body), func() {
				if mode == "canceled" {
					cancel()
				}
			}}
			err := content.WriteVerified(ctx, i, content.ResourcePacks, "pack.zip", reader, size, sha512.New(), digest)
			if err == nil || mode == "canceled" && !errors.Is(err, context.Canceled) {
				t.Fatalf("accepted %s: %v", mode, err)
			}
			entries, err := os.ReadDir(filepath.Join(i.GameDir, "resourcepacks"))
			if err != nil || len(entries) != 0 {
				t.Fatalf("failed resource left files: %v, %v", entries, err)
			}
		})
	}
}

func TestWriteVerifiedPreservesDisabledResources(t *testing.T) {
	for _, late := range []bool{false, true} {
		t.Run(fmt.Sprintf("created during copy=%t", late), func(t *testing.T) {
			i := instance.Instance{ID: 7, GameDir: t.TempDir()}
			dir := filepath.Join(i.GameDir, "resourcepacks")
			if err := os.Mkdir(dir, 0755); err != nil {
				t.Fatal(err)
			}
			disabled := filepath.Join(dir, "pack.zip.disabled")
			create := func() {
				if err := os.WriteFile(disabled, []byte("user disabled this file"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if !late {
				create()
			}
			body := resourceArchive(t, "pack.mcmeta")
			read := false
			reader := observedReader{bytes.NewReader(body), func() {
				if !read && late {
					create()
				}
				read = true
			}}
			if err := content.WriteVerified(t.Context(), i, content.ResourcePacks, "pack.zip", reader, int64(len(body)), nil, ""); err == nil {
				t.Fatal("disabled resource ignored")
			}
			if read != late {
				t.Fatalf("read file with existing disabled resource: %t", read)
			}
			data, err := os.ReadFile(disabled)
			if err != nil || string(data) != "user disabled this file" {
				t.Fatalf("disabled resource changed: %q, %v", data, err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 {
				t.Fatalf("unexpected files: %v, %v", entries, err)
			}
		})
	}
}

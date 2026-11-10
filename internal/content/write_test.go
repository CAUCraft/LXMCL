package content_test

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

package content_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

func TestInstalledResourcePackPreviews(t *testing.T) {
	i := instance.Instance{GameDir: t.TempDir()}
	root := filepath.Join(i.GameDir, "resourcepacks")
	dir := filepath.Join(root, "本地目录")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	write := func(name string, data []byte) {
		t.Helper()
		if err := os.WriteFile(name, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(dir, "pack.mcmeta"), []byte(`{"pack":{"description":"目录说明","pack_format":34,"supported_formats":{"min_inclusive":32,"max_inclusive":34}}}`))
	var cover, archive bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{G: 220, A: 255})
	if err := png.Encode(&cover, img); err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(&archive)
	for name, data := range map[string][]byte{
		"pack.mcmeta": []byte(`{"pack":{"description":[{"text":"白露\n","extra":[{"text":"秋分","bold":true}]}],"min_format":[69,1],"max_format":72}}`),
		"pack.png":    cover.Bytes(),
	} {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(root, "四季.zip.disabled"), archive.Bytes())
	if err := os.Mkdir(filepath.Join(root, "unrelated"), 0755); err != nil {
		t.Fatal(err)
	}
	files, err := content.Inventory(t.Context(), i, content.ResourcePacks)
	if err != nil || len(files) != 2 {
		t.Fatal(files, err)
	}
	for _, f := range files {
		p, err := content.PreviewResourcePack(t.Context(), i, f)
		if err != nil || p.MetadataErr != nil || p.CoverErr != nil {
			t.Fatal(p, err)
		}
		if f.Directory {
			if f.Name != "本地目录" || p.Cover != nil || p.Description != "目录说明" || p.Format != "34" || p.Supported != "32–34" {
				t.Fatal(f, p)
			}
			write(filepath.Join(dir, "pack.mcmeta"), []byte("{"))
			p, err = content.PreviewResourcePack(t.Context(), i, f)
			if err != nil || p.MetadataErr == nil || p.Description != "" {
				t.Fatal("invalid metadata retained a description", p, err)
			}
		} else if f.Enabled || p.Cover == nil || p.Description != "白露\n秋分" || p.Min != "69.1" || p.Max != "72" || p.Format != "" {
			t.Fatal(f, p)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err = content.PreviewResourcePack(ctx, i, files[0]); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

package javaruntime

import (
	"context"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

type release struct {
	Manifest transfer.Artifact
}
type runtimeFile struct {
	Type       string
	Executable bool
	Downloads  struct{ Raw transfer.Artifact }
}
type manifest struct{ Files map[string]runtimeFile }

func readManifest(path string) (manifest, error) {
	var result manifest
	f, err := os.Open(path)
	if err != nil {
		return result, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, metadataLimit+1))
	if err != nil {
		return result, err
	}
	if len(data) > metadataLimit {
		return result, i18n.Error("metadata.response_too_large")
	}
	err = json.Unmarshal(data, &result)
	return result, err
}

func (m manifest) validate() error {
	if len(m.Files) == 0 || len(m.Files) > 20000 {
		return i18n.Error("java.runtime_manifest_invalid")
	}
	seen := make(map[string]string)
	var size int64
	for name, f := range m.Files {
		key := strings.ToLower(name)
		if !storage.ValidPortablePath(name) || key == "manifest.json" || seen[key] != "" {
			return i18n.Error("java.runtime_manifest_invalid")
		}
		seen[key] = f.Type
		switch f.Type {
		case "directory":
		case "file":
			a := f.Downloads.Raw
			u, err := url.Parse(a.URL)
			if _, sumErr := a.Checksum(); err != nil || sumErr != nil || !allowedURL(u) || a.Size < 0 || a.Size > 512<<20 {
				return i18n.Error("java.runtime_manifest_invalid")
			}
			size += a.Size
			if size > 2<<30 {
				return i18n.Error("java.runtime_manifest_invalid")
			}
		default:
			return i18n.Error("java.runtime_manifest_invalid")
		}
	}
	for name := range seen {
		for parent := filepath.ToSlash(filepath.Dir(name)); parent != "."; parent = filepath.ToSlash(filepath.Dir(parent)) {
			if kind := seen[parent]; kind != "" && kind != "directory" {
				return i18n.Error("java.runtime_manifest_invalid")
			}
		}
	}
	if seen["bin/java.exe"] != "file" {
		return i18n.Error("java.runtime_manifest_invalid")
	}
	return nil
}

func (m manifest) verify(ctx context.Context, root string) error {
	for name, f := range m.Files {
		if f.Type != "file" {
			continue
		}
		a := f.Downloads.Raw
		ok, err := a.Check(ctx, filepath.Join(root, filepath.FromSlash(name)), metadataLimit)
		if err != nil {
			return err
		}
		if !ok {
			return i18n.Error("install.file_unverified", name)
		}
	}
	return ctx.Err()
}

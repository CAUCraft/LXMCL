package modpack

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/distribution"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

const receiptName = ".lxmcl-pack.json"

type receipt struct {
	Format        int
	Name, Version string
	Files         map[string]string
}

func managedFile(name string) bool {
	folder, _, nested := strings.Cut(name, "/")
	switch folder {
	case "mods", "resourcepacks", "shaderpacks", "config", "defaultconfigs", "kubejs", "scripts":
		return nested && storage.ValidPortablePath(name)
	case "options.txt":
		return !nested
	}
	return false
}

// Inspect every parent before using Root so links inside the instance cannot redirect an update.
func fileDigest(ctx context.Context, root *os.Root, name string) (string, error) {
	for dir := path.Dir(name); dir != "."; dir = path.Dir(dir) {
		st, err := root.Lstat(dir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return "", i18n.Error("modpack.archive_path_invalid", dir)
		}
	}
	st, err := root.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !st.Mode().IsRegular() {
		return "", i18n.Error("modpack.archive_path_invalid", name)
	}
	if st.Size() > distribution.MaxFileSize {
		return "oversize", nil
	}
	f, err := root.Open(name)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(transfer.Reader(ctx, f), st.Size()+1))
	if err != nil {
		return "", err
	}
	if n != st.Size() {
		return "", i18n.Error("modpack.archive_size_changed")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func writeReceipt(ctx context.Context, root *os.Root, p plan) error {
	data := receipt{1, p.info.Name, p.info.Version, make(map[string]string)}
	add := func(name string) error {
		if strings.EqualFold(name, receiptName) || strings.HasPrefix(strings.ToLower(name), receiptName+"/") {
			return i18n.Error("modpack.archive_path_invalid", name)
		}
		if !managedFile(name) || data.Files[name] != "" {
			return nil
		}
		sum, err := fileDigest(ctx, root, name)
		if err == nil {
			data.Files[name] = sum
		}
		return err
	}
	for _, d := range p.downloads {
		if err := add(d.Path); err != nil {
			return err
		}
	}
	for _, o := range p.overrides {
		if err := add(o.path); err != nil {
			return err
		}
	}
	return storage.PublishNew(ctx, root, receiptName, 0600, func(f *os.File) error { return json.NewEncoder(f).Encode(data) })
}
func readReceipt(root *os.Root) (receipt, error) {
	var data receipt
	f, err := root.Open(receiptName)
	if errors.Is(err, fs.ErrNotExist) {
		return data, nil
	}
	if err != nil {
		return data, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxBytesReceipt+1))
	if err != nil {
		return data, err
	}
	if len(b) > maxBytesReceipt || json.Unmarshal(b, &data) != nil || data.Format != 1 || len(data.Files) > maxEntries {
		return data, i18n.Error("modpack.receipt_invalid")
	}
	for name, digest := range data.Files {
		raw, err := hex.DecodeString(digest)
		if !managedFile(name) || err != nil || len(raw) != sha256.Size {
			return data, i18n.Error("modpack.receipt_invalid")
		}
	}
	return data, nil
}

const maxBytesReceipt = 32 << 20

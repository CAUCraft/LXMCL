package modpack

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

// Source identifies the archive manifest format, independent of its download provider.
type Source string

const (
	Modrinth        Source = "Modrinth"
	CurseForge      Source = "CurseForge"
	MaxArchiveSize         = storage.MaxPackSize
	maxManifestSize        = 8 << 20
	maxBytes               = storage.MaxPackBytes
	maxEntries             = storage.MaxPackEntries
)

type Info struct {
	Source                   Source
	Name, Version, Minecraft string
	Loader, LoaderVersion    string
	Downloads, Overrides     int
}

// Prepared is a comparable preview. Installation rechecks the archive fingerprint.
type Prepared struct {
	info     Info
	filename string
	feed     string
	sum      [sha256.Size]byte
}

func (p Prepared) Info() Info { return p.info }

func PrepareLocal(ctx context.Context, filename string) (Prepared, error) {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".mrpack":
		return Prepare(ctx, filename, Modrinth)
	case ".zip":
		return Prepare(ctx, filename, CurseForge)
	default:
		return Prepared{}, i18n.Error("modpack.archive_extension_invalid")
	}
}

func fingerprint(ctx context.Context, f *os.File, size int64) ([sha256.Size]byte, error) {
	var sum [sha256.Size]byte
	h := sha256.New()
	n, err := io.Copy(h, transfer.Reader(ctx, io.NewSectionReader(f, 0, size+1)))
	if err == nil && n != size {
		err = i18n.Error("modpack.archive_size_changed")
	}
	copy(sum[:], h.Sum(nil))
	return sum, err
}

// Prepare reads and validates an archive without creating an instance.
func Prepare(ctx context.Context, filename string, source Source) (Prepared, error) {
	if err := ctx.Err(); err != nil {
		return Prepared{}, err
	}
	f, size, err := storage.OpenPackFile(filename)
	if err != nil {
		return Prepared{}, err
	}
	defer f.Close()
	sum, err := fingerprint(ctx, f, size)
	if err != nil {
		return Prepared{}, err
	}
	plan, err := readPlan(f, size, source)
	if err != nil {
		return Prepared{}, err
	}
	return Prepared{info: plan.info, filename: filename, sum: sum}, ctx.Err()
}

func (p Prepared) ArchiveIdentity() (string, string) { return p.filename, hex.EncodeToString(p.sum[:]) }

// Reopen verifies a saved archive, including cache entries without a filename extension.
func Reopen(ctx context.Context, filename string, source Source, digest, feed string) (Prepared, error) {
	if !filepath.IsAbs(filename) {
		return Prepared{}, i18n.Error("modpack.archive_invalid")
	}
	if feed != "" {
		u, err := url.Parse(feed)
		if err != nil || !validFeedURL(u) {
			return Prepared{}, i18n.Error("modpack.feed_url_invalid")
		}
	}
	p, err := Prepare(ctx, filename, source)
	if err != nil {
		return Prepared{}, err
	}
	if hex.EncodeToString(p.sum[:]) != digest {
		return Prepared{}, i18n.Error("modpack.archive_changed")
	}
	p.feed = feed
	return p, nil
}

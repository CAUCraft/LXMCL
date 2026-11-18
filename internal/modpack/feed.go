package modpack

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

type feedRelease struct {
	File   string
	Size   int64
	SHA256 string
}

func (p Prepared) Feed() (string, string) {
	if p.feed == "" {
		return "", ""
	}
	return p.feed, hex.EncodeToString(p.sum[:])
}
func validFeedURL(u *url.URL) bool {
	return u != nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Fragment == ""
}
func fetchFeed(ctx context.Context, client *http.Client, u *url.URL, consume func(io.Reader) error) error {
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "CAUCraft/LXMCL")
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return &transfer.HTTPError{Status: res.StatusCode, Source: u.Host}
	}
	return consume(transfer.Reader(ctx, res.Body))
}

// PrepareFeed verifies the server's current archive before exposing it to the normal pack workflow.
func PrepareFeed(ctx context.Context, raw, cache, installed string, transport http.RoundTripper) (Prepared, bool, error) {
	origin, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !validFeedURL(origin) || !filepath.IsAbs(cache) {
		return Prepared{}, false, i18n.Error("modpack.feed_url_invalid")
	}
	sameOrigin := func(u *url.URL) bool { return validFeedURL(u) && strings.EqualFold(u.Host, origin.Host) }
	client := &http.Client{Transport: transport, Timeout: 5 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if !sameOrigin(req.URL) || len(via) >= 5 {
			return http.ErrUseLastResponse
		}
		return nil
	}}
	var release feedRelease
	err = fetchFeed(ctx, client, origin, func(r io.Reader) error {
		data, err := io.ReadAll(io.LimitReader(r, (32<<10)+1))
		if err != nil {
			return err
		}
		if len(data) > 32<<10 || json.Unmarshal(data, &release) != nil {
			return i18n.Error("modpack.feed_invalid")
		}
		return nil
	})
	if err != nil {
		return Prepared{}, false, err
	}
	digest, err := hex.DecodeString(release.SHA256)
	file, parseErr := origin.Parse(release.File)
	if err != nil || len(digest) != sha256.Size || release.Size < 1 || release.Size > MaxArchiveSize || release.File == "" || parseErr != nil || !sameOrigin(file) {
		return Prepared{}, false, i18n.Error("modpack.feed_invalid")
	}
	sum := hex.EncodeToString(digest)
	if sum == installed {
		return Prepared{}, true, nil
	}
	dir := filepath.Join(cache, "modpacks", "sha256")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return Prepared{}, false, err
	}
	name := sum + ".mrpack"
	dest := filepath.Join(dir, name)
	verify := func() (Prepared, error) {
		p, err := Prepare(ctx, dest, Modrinth)
		if err != nil {
			return Prepared{}, err
		}
		if hex.EncodeToString(p.sum[:]) != sum {
			return Prepared{}, i18n.Error("distribution.integrity_failed")
		}
		p.feed = origin.String()
		return p, nil
	}
	if st, err := os.Stat(dest); err == nil && st.Size() == release.Size {
		if p, err := verify(); err == nil {
			return p, false, nil
		}
	}
	if err := ctx.Err(); err != nil {
		return Prepared{}, false, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return Prepared{}, false, err
	}
	defer root.Close()
	var pack Prepared
	err = storage.ReplaceFile(ctx, root, name, 0600, func(f *os.File) error {
		h := sha256.New()
		err := fetchFeed(ctx, client, file, func(r io.Reader) error {
			n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(r, release.Size+1))
			if err != nil {
				return err
			}
			if n != release.Size || hex.EncodeToString(h.Sum(nil)) != sum {
				return i18n.Error("distribution.integrity_failed")
			}
			return nil
		})
		if err != nil {
			return err
		}
		plan, err := readPlan(f, release.Size, Modrinth)
		if err != nil {
			return err
		}
		pack = Prepared{info: plan.info, filename: dest, feed: origin.String()}
		copy(pack.sum[:], digest)
		return nil
	})
	if err != nil {
		return Prepared{}, false, err
	}
	return pack, false, err
}

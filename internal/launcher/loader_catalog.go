package launcher

import (
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

type LoaderRelease struct {
	Version string
	Preview bool
}

func LoaderVersions(ctx context.Context, game, kind string) ([]LoaderRelease, error) {
	return loaderVersions(ctx, game, kind, &downloads{http: officialHTTP})
}

func loaderVersions(ctx context.Context, game, kind string, d *downloads) ([]LoaderRelease, error) {
	if !instance.SupportedRelease(game) {
		return nil, i18n.Error("version.select_supported")
	}
	var versions []string
	previews := make(map[string]bool)
	switch kind {
	case LoaderVanilla:
		return nil, nil
	case LoaderFabric, LoaderQuilt:
		source := fabricMeta
		if kind == LoaderQuilt {
			source = quiltMeta
		}
		var entries []struct {
			Loader struct {
				Version string
				Stable  *bool
			}
		}
		if err := downloadJSON(ctx, d, source+game, &entries); err != nil {
			return nil, err
		}
		for _, e := range entries {
			versions = append(versions, e.Loader.Version)
			if e.Loader.Stable != nil && !*e.Loader.Stable {
				previews[e.Loader.Version] = true
			}
		}
	case LoaderForge, LoaderNeoForge:
		source, prefix := forgeMaven+"net/minecraftforge/forge/", game+"-"
		switch {
		case kind == LoaderNeoForge && game == "1.20.1":
			source = neoMaven + "net/neoforged/forge/"
		case kind == LoaderNeoForge:
			source = neoMaven + "net/neoforged/neoforge/"
			prefix = neoGamePrefix(game)
		}
		var doc struct {
			Versions []string `xml:"versioning>versions>version"`
		}
		budget := 12 * time.Second
		if kind == LoaderNeoForge {
			budget = 25 * time.Second
		}
		query, stop := context.WithTimeout(ctx, budget)
		err := d.fetch(query, source+"maven-metadata.xml", func(ctx context.Context, resp *http.Response) error {
			body, err := io.ReadAll(io.LimitReader(transfer.Reader(ctx, resp.Body), maxMetadata+1))
			if err != nil {
				return err
			}
			if len(body) > maxMetadata {
				return i18n.Error("loader.catalog_too_large")
			}
			return xml.Unmarshal(body, &doc)
		})
		stop()
		if err != nil {
			if kind == LoaderForge && ctx.Err() == nil {
				// The smaller official promotions index is useful on slow connections.
				var fallback struct{ Promos map[string]string }
				if alternate := downloadJSON(ctx, d, "https://files.minecraftforge.net/net/minecraftforge/forge/promotions_slim.json", &fallback); alternate == nil {
					versions = append(versions, fallback.Promos[game+"-recommended"], fallback.Promos[game+"-latest"])
					break
				}
			}
			return nil, err
		}
		for _, version := range doc.Versions {
			if !strings.HasPrefix(version, prefix) {
				continue
			}
			if kind == LoaderForge || game == "1.20.1" {
				version = strings.TrimPrefix(version, prefix)
			}
			versions = append(versions, version)
		}
	default:
		return nil, i18n.Error("loader.catalog_kind_unsupported")
	}
	var result []LoaderRelease
	seen := make(map[string]bool)
	for _, version := range versions {
		_, v, err := instance.NormalizeLoader(game, kind, version)
		if err != nil || seen[v] {
			continue
		}
		seen[v] = true
		result = append(result, LoaderRelease{v, previews[v] || strings.Contains(v, "-")})
	}
	slices.SortFunc(result, func(a, b LoaderRelease) int {
		if a.Preview != b.Preview {
			if a.Preview {
				return 1
			}
			return -1
		}
		return compareLoaderVersions(b.Version, a.Version)
	})
	return result, nil
}

func neoGamePrefix(game string) string {
	parts := strings.Split(game, ".")
	if len(parts) == 2 {
		parts = append(parts, "0")
	}
	if parts[0] == "1" {
		parts = parts[1:]
	}
	return strings.Join(parts, ".") + "."
}

func compareLoaderVersions(a, b string) int {
	parts := func(s string) []string {
		return strings.FieldsFunc(s, func(r rune) bool { return r == '.' || r == '-' || r == '+' })
	}
	x, y := parts(a), parts(b)
	for n := range min(len(x), len(y)) {
		if x[n] == y[n] {
			continue
		}
		a, aErr := strconv.Atoi(x[n])
		b, bErr := strconv.Atoi(y[n])
		if aErr == nil && bErr == nil {
			if a != b {
				return a - b
			}
		} else {
			return strings.Compare(x[n], y[n])
		}
	}
	return strings.Compare(a, b)
}

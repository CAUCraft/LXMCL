// Package packmeta reads the format declarations and plain-text description of pack.mcmeta.
package packmeta

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/mctext"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

const Filename = "pack.mcmeta"
const MaxBytes = 512 << 10

type Metadata struct {
	Description, Format, Supported, Min, Max string
}

// Read reads the selected metadata payload only, within the manifest limit.
func Read(ctx context.Context, r io.Reader) (Metadata, error) {
	data, err := io.ReadAll(io.LimitReader(transfer.Reader(ctx, r), MaxBytes+1))
	if err != nil {
		return Metadata{}, err
	}
	return Decode(data)
}

func Decode(data []byte) (Metadata, error) {
	if len(data) > MaxBytes {
		return Metadata{}, i18n.Error("resource.filename_or_size_invalid")
	}
	var p Metadata
	err := p.decode(data)
	return p, err
}

func (p *Metadata) decode(data []byte) error {
	var meta struct{ Pack map[string]any }
	if err := json.Unmarshal(data, &meta); err != nil {
		return err
	}
	if meta.Pack == nil {
		return i18n.Error("resource.pack_metadata_invalid")
	}
	p.Description = mctext.Component(meta.Pack["description"], 4096)
	p.Format = formatNumber(meta.Pack["pack_format"])
	p.Min = formatVersion(meta.Pack["min_format"])
	p.Max = formatVersion(meta.Pack["max_format"])
	switch s := meta.Pack["supported_formats"].(type) {
	case []any:
		if len(s) == 2 {
			p.Supported = formatRange(s[0], s[1])
		}
	case map[string]any:
		p.Supported = formatRange(s["min_inclusive"], s["max_inclusive"])
	default:
		p.Supported = formatNumber(s)
	}
	return nil
}

func formatNumber(v any) string {
	n, ok := v.(float64)
	if !ok || n < 0 || n > math.MaxInt32 || n != math.Trunc(n) {
		return ""
	}
	return strconv.Itoa(int(n))
}

func formatVersion(v any) string {
	if pair, ok := v.([]any); ok {
		if len(pair) < 1 || len(pair) > 2 {
			return ""
		}
		var parts []string
		for _, value := range pair {
			n := formatNumber(value)
			if n == "" {
				return ""
			}
			parts = append(parts, n)
		}
		return strings.Join(parts, ".")
	}
	return formatNumber(v)
}

func formatRange(lo, hi any) string {
	a, b := formatNumber(lo), formatNumber(hi)
	if a == "" || b == "" {
		return ""
	}
	return a + "–" + b
}

package transfer_test

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/transfer"
)

func TestArtifactUsesTheSameSizeBoundForCacheAndDownload(t *testing.T) {
	const data = "verified bytes"
	a := transfer.Artifact{SHA: fmt.Sprintf("%x", sha1.Sum([]byte(data))), Size: transfer.UnknownSize}
	dir := t.TempDir()
	file := filepath.Join(dir, "artifact")
	calls := 0
	fetch := func(ctx context.Context, _ string, consume func(context.Context, io.Reader) error) error {
		calls++
		return consume(ctx, strings.NewReader(data))
	}
	if err := a.Ensure(t.Context(), file, int64(len(data)-1), fetch); !errors.Is(err, transfer.ErrMismatch) {
		t.Fatalf("oversized download: %v", err)
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("failed download left files: %v, %v", entries, err)
	}
	if err := a.Ensure(t.Context(), file, int64(len(data)), fetch); err != nil {
		t.Fatalf("download at the exact bound: %v", err)
	}
	if ok, err := a.Check(t.Context(), file, int64(len(data)-1)); ok || err != nil {
		t.Fatalf("oversized cached file: %v, %v", ok, err)
	}
	if ok, err := a.Check(t.Context(), file, int64(len(data))); !ok || err != nil {
		t.Fatalf("cached file at the exact bound: %v, %v", ok, err)
	}
	for _, limit := range []int64{-1, math.MaxInt64} {
		if ok, err := a.Check(t.Context(), file, limit); ok || err == nil {
			t.Fatalf("check accepted invalid bound %d: %v, %v", limit, ok, err)
		}
		if err := a.Ensure(t.Context(), file, limit, fetch); err == nil {
			t.Fatalf("ensure accepted invalid bound %d", limit)
		}
	}
	if calls != 2 {
		t.Fatalf("invalid metadata triggered a download: %d calls", calls)
	}
	// A known size is authoritative even when it exceeds the unknown-size bound.
	a.Size = int64(len(data))
	if ok, err := a.Check(t.Context(), file, 0); !ok || err != nil {
		t.Fatalf("known size was limited by unknownLimit: %v, %v", ok, err)
	}
}

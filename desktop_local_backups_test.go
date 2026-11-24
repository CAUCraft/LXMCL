package main

import (
	"slices"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/saves"
)

func TestFilterLocalBackupFilenamesPreservesEntriesAndOrder(t *testing.T) {
	newest := saves.BackupEntry{Name: "20260310-010101-ABC123.zip", Size: 9}
	missing := saves.BackupEntry{Name: "20260309-010101-abc456.zip", Missing: true}
	other := saves.BackupEntry{Name: "20260308-010101-def789.zip", Size: 4}
	entries := []saves.BackupEntry{newest, missing, other}
	p := localBackupView{entries: slices.Clone(entries)}
	for _, tc := range []struct {
		query string
		want  []saves.BackupEntry
	}{
		{"  AbC  ", []saves.BackupEntry{newest, missing}},
		{"unmatched", nil},
		{"", entries},
	} {
		p.query = tc.query
		if got := p.visibleEntries(); !slices.Equal(got, tc.want) {
			t.Errorf("query %q selected %v, want %v", tc.query, got, tc.want)
		}
	}
	if !slices.Equal(p.entries, entries) {
		t.Fatalf("filter modified the source list: %v", p.entries)
	}
}

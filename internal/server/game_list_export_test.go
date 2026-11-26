package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestExportGameList(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "servers.dat")
	rows := []Server{
		{Name: "河谷\x00🏰", Address: "Legacy.Example.INVALID:25565"},
		{Name: "建筑", Address: "[2001:db8::2]:25566"},
	}
	if err := ExportGameList(t.Context(), target, nil); err == nil {
		t.Fatal("empty selection exported")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := ExportGameList(ctx, target, rows); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled export", err)
	}
	if _, err := os.Stat(target); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("empty selection or canceled export left a file", err)
	}
	if err := ExportGameList(t.Context(), target, rows); err != nil {
		t.Fatal(err)
	}
	got, err := ReadGameList(t.Context(), dir)
	if err != nil || !slices.Equal(got, rows) {
		t.Fatal(got, err)
	}

	// Verify the wire format independently of the reader's lenient UTF decoder.
	want := []byte{10, 0, 0, 9, 0, 7, 's', 'e', 'r', 'v', 'e', 'r', 's', 10, 0, 0, 0, 2}
	putString := func(s string) {
		want = append(binary.BigEndian.AppendUint16(want, uint16(len(s))), s...)
	}
	for i, row := range rows {
		want = append(want, 8)
		putString("name")
		putString([]string{"河谷\xc0\x80\xed\xa0\xbc\xed\xbf\xb0", "建筑"}[i])
		want = append(want, 8)
		putString("ip")
		putString(row.Address)
		want = append(want, 0)
	}
	want = append(want, 0)
	if err := ExportGameList(t.Context(), target, rows[1:]); !errors.Is(err, fs.ErrExist) {
		t.Fatal("existing output not protected", err)
	}
	if wire, err := os.ReadFile(target); err != nil || !bytes.Equal(wire, want) {
		t.Fatalf("unexpected server list bytes: %x, %v", wire, err)
	}
}

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestFavoritesBackupRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "favorites.json")
	long := strings.Repeat("abc.", 22) + "example.invalid:25565"
	rows := []Server{{Name: "河谷", Address: "[2001:db8::7]:25573"}, {Name: long, Address: long}}
	if err := ExportFavorites(context.Background(), path, rows); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ReadFavorites(path)
	if err != nil || !slices.Equal(got, rows) {
		t.Fatal(got, err)
	}
	if err := ExportFavorites(context.Background(), path, rows[:1]); !errors.Is(err, fs.ErrExist) {
		t.Fatal("collision", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, after) {
		t.Fatal("existing backup changed", err)
	}
	bad := favoritesBackup{Format: favoritesFormat, Servers: append(rows, Server{Name: "bad", Address: ""})}
	mixed, err := json.Marshal(bad)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range [][]byte{data[:len(data)/2], []byte(`{"servers":[]}`), mixed} {
		if err := os.WriteFile(path, input, 0600); err != nil {
			t.Fatal(err)
		}
		if got, err := ReadFavorites(path); err == nil || got != nil {
			t.Fatal("invalid backup yielded rows", got, err)
		}
	}
}

package server

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestReadGameList(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "servers.dat")
	if _, err := ReadGameList(t.Context(), dir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("missing list", err)
	}
	var data bytes.Buffer
	str := func(s string) { data.Write(binary.BigEndian.AppendUint16(nil, uint16(len(s)))); data.WriteString(s) }
	data.Write([]byte{10, 0, 0, 3})
	str("DataVersion")
	data.Write([]byte{0, 0, 15, 0, 9})
	str("servers")
	data.Write([]byte{10, 0, 0, 0, 3})
	for n, addr := range []string{"one.example.invalid:25572", "hidden.example.invalid", "[2001:db8::2]:25566"} {
		data.WriteByte(8)
		str("ip")
		str(addr)
		data.WriteByte(7)
		str("icon")
		data.Write([]byte{0, 0, 0, 3, 1, 2, 3})
		data.WriteByte(8)
		str("name")
		str([]string{"河谷", "最近连接", "建筑\xed\xa0\xbc\xed\xbf\xb0"}[n])
		if n == 1 {
			data.WriteByte(1)
			str("hidden")
			data.WriteByte(1)
		}
		data.WriteByte(0)
	}
	data.WriteByte(0)
	if err := os.WriteFile(file, data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadGameList(t.Context(), dir)
	if err != nil || !slices.Equal(got, []Server{{Name: "河谷", Address: "one.example.invalid:25572"}, {Name: "建筑🏰", Address: "[2001:db8::2]:25566"}}) {
		t.Fatalf("%+v %v", got, err)
	}
	if err := os.WriteFile(file, data.Bytes()[:data.Len()-1], 0600); err != nil {
		t.Fatal(err)
	}
	if rows, err := ReadGameList(t.Context(), dir); err == nil || len(rows) != 0 {
		t.Fatal("partial list", rows, err)
	}
	empty := []byte{10, 0, 0, 9, 0, 7, 's', 'e', 'r', 'v', 'e', 'r', 's', 0, 0, 0, 0, 0, 0}
	if err := os.WriteFile(file, empty, 0600); err != nil {
		t.Fatal(err)
	}
	if rows, err := ReadGameList(t.Context(), dir); err != nil || len(rows) != 0 {
		t.Fatal("empty list", rows, err)
	}
}

package storage

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestReadArchiveJSONIsBoundedAndRejectsTrailingData(t *testing.T) {
	for _, tc := range []struct {
		body  string
		limit int64
		valid bool
	}{
		{`{"value":1}`, 100, true},
		{`{"value":1} garbage`, 100, false},
		{`{"value":1}`, 4, false},
	} {
		var buf bytes.Buffer
		w := zip.NewWriter(&buf)
		entry, err := w.Create("manifest.json")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(tc.body)); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		z, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		if err != nil {
			t.Fatal(err)
		}
		var out struct{ Value int }
		err = ReadArchiveJSON(z.File[0], tc.limit, &out)
		if (err == nil) != tc.valid {
			t.Fatalf("%q: %v", tc.body, err)
		}
	}
	if err := ReadArchiveJSON(nil, 100, new(any)); err == nil {
		t.Fatal("missing entry accepted")
	}
}

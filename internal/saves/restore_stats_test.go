package saves_test

import (
	"archive/zip"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/saves"
)

func TestInspectWorldStats(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entries map[string]string
		want    map[string]saves.WorldStats
	}{
		{
			name: "wrapped",
			entries: map[string]string{
				"saves/":                          "",
				"saves/河谷/":                       "",
				"saves/河谷/level.dat":              "ABC",
				"saves/河谷/empty.txt":              "",
				"saves/河谷/session.lock":           "ignored",
				"saves/creative/level.dat":        "WXYZ",
				"saves/creative/region/":          "",
				"saves/creative/region/r.0.0.mca": "42",
			},
			want: map[string]saves.WorldStats{
				"河谷":       {FileCount: 2, UncompressedBytes: 3},
				"creative": {FileCount: 2, UncompressedBytes: 6},
			},
		},
		{
			name: "root",
			entries: map[string]string{
				"level.dat":        "",
				"region/":          "",
				"region/empty.bin": "",
				"session.lock":     "ignored",
			},
			want: map[string]saves.WorldStats{"root": {FileCount: 2, UncompressedBytes: 0}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			filename := filepath.Join(t.TempDir(), tc.name+".zip")
			file, err := os.Create(filename)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { file.Close() })
			archive := zip.NewWriter(file)
			for name, body := range tc.entries {
				entry, err := archive.Create(name)
				if err != nil {
					t.Fatal(err)
				}
				if body != "" {
					if _, err := entry.Write([]byte(body)); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := archive.Close(); err != nil {
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			preview, err := saves.Inspect(t.Context(), filename)
			if err != nil {
				t.Fatal(err)
			}
			stats := preview.WorldStats()
			if !maps.Equal(stats, tc.want) {
				t.Fatalf("got %+v, want %+v", stats, tc.want)
			}
			clear(stats)
			if !maps.Equal(preview.WorldStats(), tc.want) {
				t.Fatal("caller could change preview statistics")
			}
		})
	}
	if stats := (saves.Preview{}).WorldStats(); stats != nil {
		t.Fatal("an unavailable preview has statistics", stats)
	}
}

package saves_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CAUCraft/LXMCL/internal/saves"
)

func TestRecentShowsOnlyNewestLocalArchives(t *testing.T) {
	root := t.TempDir()
	dir := saves.BackupsDir(root, 2)
	if got, err := saves.Recent(t.Context(), root, 2); err != nil || len(got) != 0 {
		t.Fatalf("new instance: %v %v", got, err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for index, name := range []string{"first.zip", "second.zip", "third.zip", "last.ZIP", "unpublished.tmp"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
		modified := time.Unix(int64(index+1)*3600+1700000000, 0)
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "directory.zip"), 0700); err != nil {
		t.Fatal(err)
	}
	got, err := saves.Recent(t.Context(), root, 2)
	if err != nil || len(got) != 3 || got[0].Name != "last.ZIP" || got[1].Name != "third.zip" ||
		got[2].Name != "second.zip" || got[0].Size != int64(len(got[0].Name)) {
		t.Fatalf("recent archives: %v %v", got, err)
	}
}

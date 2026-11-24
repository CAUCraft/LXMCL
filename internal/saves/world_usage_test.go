package saves_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/saves"
)

func TestWorldFileStatsUseOnlySelectedBackupFiles(t *testing.T) {
	source, target, world := copyWorldFixture(t)
	worlds := filepath.Join(source.GameDir, "saves")
	writeCopyWorldFile(t, worlds, world.Folder(), "empty.txt", "")
	if err := os.Mkdir(filepath.Join(worlds, world.Folder(), "empty-folder"), 0700); err != nil {
		t.Fatal(err)
	}
	writeCopyWorldFile(t, worlds, "other-world", "level.dat", "not counted")
	writeCopyWorldFile(t, filepath.Join(target.GameDir, "saves"), world.Folder(), "level.dat", "different instance")
	want := saves.WorldFileStats{FileCount: 3, Bytes: uint64(len("source level") + len("source region"))}
	got, err := world.BackupStats(t.Context(), source.GameDir)
	if err != nil || got != want {
		t.Fatalf("statistics: %+v %v, want %+v", got, err, want)
	}
	if crossed, err := world.BackupStats(t.Context(), target.GameDir); err == nil || crossed != (saves.WorldFileStats{}) {
		t.Fatal("counted the other instance's same-name world", crossed, err)
	}

	archive, err := saves.BackupWorld(t.Context(), source.GameDir, t.TempDir(), source.ID, world)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := saves.Inspect(t.Context(), archive)
	if err != nil {
		t.Fatal(err)
	}
	contents := preview.WorldStats()[world.Folder()]
	if contents.FileCount != got.FileCount || contents.UncompressedBytes != got.Bytes {
		t.Fatal("statistics disagree with backup scope", contents, got)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	canceledDuringScan := &cancelWorldStats{Context: ctx, cancel: cancel, remaining: 6}
	if partial, err := world.BackupStats(canceledDuringScan, source.GameDir); !errors.Is(err, context.Canceled) || partial != (saves.WorldFileStats{}) {
		t.Fatal("returned partial statistics after cancellation", partial, err)
	}
}

type cancelWorldStats struct {
	context.Context
	cancel    context.CancelFunc
	remaining int
}

func (c *cancelWorldStats) Err() error {
	if c.remaining > 0 {
		c.remaining--
		if c.remaining == 0 {
			c.cancel()
		}
	}
	return c.Context.Err()
}

func TestWorldFileStatsRejectLinkedDirectory(t *testing.T) {
	source, _, world := copyWorldFixture(t)
	worldPath := filepath.Join(source.GameDir, "saves", world.Folder())
	if err := os.Symlink(filepath.Join(worldPath, "region"), filepath.Join(worldPath, "linked-region")); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	if partial, err := world.BackupStats(t.Context(), source.GameDir); err == nil || partial != (saves.WorldFileStats{}) {
		t.Fatal("treated an unreadable backup plan as complete", partial, err)
	}
}

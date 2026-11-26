package content_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

func TestCompareResourcePacks(t *testing.T) {
	current := instance.Instance{ID: 1, Name: "同名实例", GameDir: t.TempDir()}
	other := instance.Instance{ID: 2, Name: current.Name, GameDir: t.TempDir()}
	write := func(i instance.Instance, path string) {
		t.Helper()
		name := filepath.Join(i.GameDir, string(content.ResourcePacks), filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte("inventory marker"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	check := func(want ...content.ResourcePackMatch) {
		t.Helper()
		got, err := content.CompareResourcePacks(t.Context(), current, other)
		if err != nil || !slices.Equal(got, want) {
			t.Fatalf("matches = %+v, %v; want %+v", got, err, want)
		}
	}
	check() // Existing game directories without a resourcepacks directory are empty.
	for _, name := range []string{"Case.zip", "Flip.zip.disabled", "Both.zip", "Both.zip.disabled", "Shared/pack.mcmeta", "Type.zip/pack.mcmeta", "NotAPack/readme.txt"} {
		write(current, name)
	}
	for _, name := range []string{"case.zip", "Flip.zip", "Both.zip", "Shared/pack.mcmeta", "Type.zip"} {
		write(other, name)
	}
	present, absent, disabled, duplicate := content.PackPresent, content.PackAbsent, content.PackDisabled, content.PackDuplicate
	check(
		content.ResourcePackMatch{Name: "Both.zip", Current: duplicate, Other: present},
		content.ResourcePackMatch{Name: "Case.zip", Current: present, Other: absent},
		content.ResourcePackMatch{Name: "Flip.zip", Current: disabled, Other: present},
		content.ResourcePackMatch{Name: "Shared", Directory: true, Current: present, Other: present},
		content.ResourcePackMatch{Name: "Type.zip", Directory: true, Current: present, Other: absent},
		content.ResourcePackMatch{Name: "Type.zip", Current: absent, Other: present},
		content.ResourcePackMatch{Name: "case.zip", Current: absent, Other: present},
	)
	checkFailure := func(i instance.Instance) error {
		t.Helper()
		got, err := content.CompareResourcePacks(t.Context(), current, i)
		var scan *content.ResourcePackScanError
		if got != nil || !errors.As(err, &scan) || scan.InstanceID != i.ID {
			t.Fatalf("matches = %+v, error = %v; want a scan failure for #%d", got, err, i.ID)
		}
		return err
	}
	missing := instance.Instance{ID: other.ID, Name: other.Name, GameDir: filepath.Join(t.TempDir(), "missing")}
	if err := checkFailure(missing); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing game root: %v", err)
	}
	invalid := instance.Instance{ID: other.ID, Name: other.Name, GameDir: t.TempDir()}
	if err := os.WriteFile(filepath.Join(invalid.GameDir, string(content.ResourcePacks)), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	checkFailure(invalid)
}

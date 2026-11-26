package content

import (
	"reflect"
	"testing"
)

func TestDuplicateEnabledModIDs(t *testing.T) {
	files := []File{
		{Name: "z.jar", Enabled: true, mods: []manifestMod{
			{id: "shared", version: "2.0"}, {id: "shared", version: "2.0"}, {id: "zeta", version: "1.0"},
		}},
		{Name: "b.jar", Enabled: true, mods: []manifestMod{
			{id: "shared", version: "1.0"}, {id: "shared", version: "9.0"}, {id: "zeta", version: "${file.jarVersion}"},
		}},
		{Name: "a.jar", Enabled: true, mods: []manifestMod{{id: "shared", version: "0.9"}}},
		{Name: "disabled.jar", mods: []manifestMod{{id: "shared", version: "99.0"}, {id: "alone"}}},
		{Name: "only.jar", Enabled: true, mods: []manifestMod{{id: "alone"}, {id: "alone"}}},
		{Name: "upper.jar", Enabled: true, mods: []manifestMod{{id: "Shared"}}},
		{Name: "spaced.jar", Enabled: true, mods: []manifestMod{{id: " shared "}}},
		{Name: "unknown.jar", Enabled: true, mods: []manifestMod{{}, {id: " "}}},
		{Name: "shared.jar", Enabled: true, ModIDs: "shared"},
		{Name: "directory", Enabled: true, Directory: true, mods: []manifestMod{{id: "shared"}}},
	}
	want := []DuplicateModID{
		{ID: "shared", Files: []ModIDFile{{Name: "a.jar", Version: "0.9"}, {Name: "b.jar"}, {Name: "z.jar", Version: "2.0"}}},
		{ID: "zeta", Files: []ModIDFile{{Name: "b.jar"}, {Name: "z.jar", Version: "1.0"}}},
	}
	if got := DuplicateEnabledModIDs(files); !reflect.DeepEqual(got, want) {
		t.Fatalf("duplicate IDs = %#v, want %#v", got, want)
	}
	if got := DuplicateEnabledModIDs([]File{files[0], files[0], files[3]}); len(got) != 0 {
		t.Fatalf("counted the same file or a disabled file: %#v", got)
	}
}

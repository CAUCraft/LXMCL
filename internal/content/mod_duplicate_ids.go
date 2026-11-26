package content

import (
	"cmp"
	"slices"
	"strings"
)

type ModIDFile struct{ Name, Version string }

type DuplicateModID struct {
	ID    string
	Files []ModIDFile
}

// DuplicateEnabledModIDs groups exact IDs from the current inventory snapshot.
// A declaration counts once per enabled file, even if repeated in that file.
func DuplicateEnabledModIDs(files []File) []DuplicateModID {
	byID := make(map[string]map[string]string)
	for _, file := range files {
		if !file.Enabled || file.Directory {
			continue
		}
		name := file.DiskName()
		for _, mod := range file.mods {
			if strings.TrimSpace(mod.id) == "" {
				continue
			}
			versions := byID[mod.id]
			if versions == nil {
				versions = make(map[string]string)
				byID[mod.id] = versions
			}
			version := mod.knownVersion()
			if previous, found := versions[name]; found && previous != version {
				version = ""
			}
			versions[name] = version
		}
	}

	var duplicates []DuplicateModID
	for id, versions := range byID {
		if len(versions) < 2 {
			continue
		}
		group := DuplicateModID{ID: id, Files: make([]ModIDFile, 0, len(versions))}
		for name, version := range versions {
			group.Files = append(group.Files, ModIDFile{Name: name, Version: version})
		}
		slices.SortFunc(group.Files, func(a, b ModIDFile) int { return cmp.Compare(a.Name, b.Name) })
		duplicates = append(duplicates, group)
	}
	slices.SortFunc(duplicates, func(a, b DuplicateModID) int { return cmp.Compare(a.ID, b.ID) })
	return duplicates
}

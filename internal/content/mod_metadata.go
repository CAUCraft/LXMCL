package content

import (
	"archive/zip"
	"encoding/json"
	"io"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// ReadModMetadata returns a display title and comma-separated mod IDs from loader manifests.
// A broken or unfamiliar JAR returns empty strings.
func ReadModMetadata(root *os.Root, name string, markers []string) (string, string) {
	return modLabels(readManifestMods(root, name, markers))
}

type manifestMod struct{ id, name, version string }

func modLabels(entries []manifestMod) (string, string) {
	var titles, ids []string
	for _, m := range entries {
		titles = append(titles, modTitle(m.id, m.name, m.version))
		ids = append(ids, m.id)
	}
	return strings.Join(titles, " / "), strings.Join(ids, ", ")
}

func readManifestMods(root *os.Root, name string, markers []string) []manifestMod {
	f, err := root.Open(name)
	if err != nil {
		return nil
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() > MaxSize {
		return nil
	}
	return manifestMods(f, st.Size(), markers)
}

func manifestMods(f io.ReaderAt, size int64, markers []string) []manifestMod {
	z, err := zip.NewReader(f, size)
	if err != nil {
		return nil
	}
	for _, marker := range markers {
		data := manifestBytes(z, marker)
		if len(data) == 0 {
			continue
		}
		switch marker {
		case "fabric.mod.json":
			var m struct{ ID, Name, Version string }
			if json.Unmarshal(data, &m) == nil && m.ID != "" {
				return []manifestMod{{m.ID, m.Name, m.Version}}
			}
		case "quilt.mod.json":
			var m struct {
				Loader struct {
					ID, Version string
					Metadata    struct{ Name string }
				} `json:"quilt_loader"`
			}
			if json.Unmarshal(data, &m) == nil && m.Loader.ID != "" {
				return []manifestMod{{m.Loader.ID, m.Loader.Metadata.Name, m.Loader.Version}}
			}
		default:
			var m struct {
				Mods []struct{ ModID, DisplayName, Version string }
			}
			if err := toml.Unmarshal(data, &m); err != nil {
				continue
			}
			var entries []manifestMod
			var jarVersion string
			var readJarVersion bool
			for _, mod := range m.Mods {
				if mod.ModID == "" {
					continue
				}
				version := mod.Version
				if version == "${file.jarVersion}" {
					if !readJarVersion {
						readJarVersion = true
						jarVersion = implementationVersion(z)
					}
					version = jarVersion
				}
				entries = append(entries, manifestMod{mod.ModID, mod.DisplayName, version})
			}
			if len(entries) > 0 {
				return entries
			}
		}
	}
	return nil
}

func modTitle(id, name, version string) string {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		name = id
	}
	if version = strings.TrimSpace(version); version != "" {
		return name + " · " + version
	}
	return name
}

func manifestBytes(z *zip.Reader, name string) []byte {
	const limit = 512 << 10
	for _, entry := range z.File {
		if entry.Name != name {
			continue
		}
		if entry.UncompressedSize64 > limit {
			return nil
		}
		r, err := entry.Open()
		if err != nil {
			return nil
		}
		defer r.Close()
		data, err := io.ReadAll(io.LimitReader(r, limit+1))
		if err == nil && len(data) <= limit {
			return data
		}
		return nil
	}
	return nil
}

func implementationVersion(z *zip.Reader) string {
	text := strings.ReplaceAll(string(manifestBytes(z, "META-INF/MANIFEST.MF")), "\r\n", "\n")
	main, _, _ := strings.Cut(text, "\n\n")
	for line := range strings.SplitSeq(strings.ReplaceAll(main, "\n ", ""), "\n") {
		key, value, found := strings.Cut(line, ": ")
		if found && strings.EqualFold(key, "Implementation-Version") {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

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
	f, err := root.Open(name)
	if err != nil {
		return "", ""
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() > MaxSize {
		return "", ""
	}
	z, err := zip.NewReader(f, st.Size())
	if err != nil {
		return "", ""
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
				return modTitle(m.ID, m.Name, m.Version), m.ID
			}
		case "quilt.mod.json":
			var m struct {
				Loader struct {
					ID, Version string
					Metadata    struct{ Name string }
				} `json:"quilt_loader"`
			}
			if json.Unmarshal(data, &m) == nil && m.Loader.ID != "" {
				return modTitle(m.Loader.ID, m.Loader.Metadata.Name, m.Loader.Version), m.Loader.ID
			}
		default:
			var m struct {
				Mods []struct{ ModID, DisplayName, Version string }
			}
			if err := toml.Unmarshal(data, &m); err != nil {
				continue
			}
			var titles, ids []string
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
				titles = append(titles, modTitle(mod.ModID, mod.DisplayName, version))
				ids = append(ids, mod.ModID)
			}
			if len(ids) > 0 {
				return strings.Join(titles, " / "), strings.Join(ids, ", ")
			}
		}
	}
	return "", ""
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

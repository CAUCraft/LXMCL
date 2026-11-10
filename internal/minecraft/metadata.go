package minecraft

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

type rule struct {
	Action   string
	Features map[string]bool
	OS       struct{ Name, Arch, Version string }
}

type metadata struct {
	ID, Type, MainClass string
	Arguments           struct{ JVM, Game []json.RawMessage }
	JavaVersion         struct {
		MajorVersion int
		Component    string
	}
	Downloads  struct{ Client artifact }
	AssetIndex struct {
		ID string
		artifact
	}
	Libraries []struct {
		Name      string
		Rules     []rule
		Downloads struct{ Artifact artifact }
		Natives   map[string]string
	}
	Logging struct {
		Client struct {
			Argument string
			File     struct {
				ID string
				artifact
			}
		}
	}
}

type assetIndex struct {
	Objects map[string]struct {
		Hash string
		Size int64
	}
}

func (g Runtime) versionDir(id string) string { return filepath.Join(g.Cache, "versions", id) }

func (g Runtime) readVersion(id, expectedSHA string) (metadata, error) {
	var m metadata
	f, err := os.Open(filepath.Join(g.versionDir(id), "version.json"))
	if err != nil {
		return m, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxMetadata+1))
	if err != nil {
		return m, err
	}
	sum := sha1.Sum(b)
	if len(b) > maxMetadata || (expectedSHA != "" && !strings.EqualFold(hex.EncodeToString(sum[:]), expectedSHA)) {
		return m, i18n.Error("metadata.checksum_failed")
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return m, i18n.Error("metadata.read_failed", err)
	}
	if m.ID != id || m.Type != "release" || m.MainClass == "" || len(m.Arguments.JVM) == 0 || m.JavaVersion.MajorVersion < 17 {
		return m, i18n.Error("metadata.version_invalid")
	}
	return m, nil
}

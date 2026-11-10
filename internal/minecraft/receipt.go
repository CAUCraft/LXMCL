package minecraft

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

type installReceipt struct {
	Platform   string `json:"platform"`
	VersionSHA string `json:"version_sha1"`
}

func platformKey() string { return runtime.GOOS + "/" + runtime.GOARCH }

func (g Runtime) readReceipt(id string) (installReceipt, error) {
	var r installReceipt
	f, err := os.Open(filepath.Join(g.versionDir(id), "verified"))
	if err != nil {
		return r, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 1025))
	if err != nil {
		return r, err
	}
	if len(b) > 1024 {
		return r, i18n.Error("install.receipt_corrupt")
	}
	// Existing installations did not persist the catalog SHA-1. Continue checking
	// their complete artifacts, and upgrade the receipt after an explicit repair.
	if strings.TrimSpace(string(b)) == "1" {
		return r, nil
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return r, i18n.Error("install.receipt_corrupt")
	}
	if r.Platform != platformKey() {
		return r, i18n.Error("install.platform_mismatch")
	}
	if _, err := (artifact{SHA: r.VersionSHA, Size: unknownSize}).Checksum(); err != nil {
		return r, i18n.Error("install.receipt_checksum_missing")
	}
	return r, nil
}

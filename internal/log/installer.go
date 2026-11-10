package log

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

func CreateInstaller(cache, loader string) (*os.File, error) {
	loader = strings.ToLower(loader)
	if loader != "forge" && loader != "neoforge" {
		return nil, i18n.Error("log.installer_unsupported")
	}
	dir := filepath.Join(cache, "installer-logs")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	return os.CreateTemp(dir, loader+"-*.log")
}

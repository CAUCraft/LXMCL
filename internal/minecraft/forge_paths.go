package minecraft

import (
	"path/filepath"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

const (
	forgeMaven = "https://maven.minecraftforge.net/"
	neoMaven   = "https://maven.neoforged.net/releases/"
)

func forgeInstallerURL(i instance.Instance) (string, error) {
	loader, version, err := instance.NormalizeLoader(i.Version, i.Loader, i.LoaderVersion)
	if err != nil {
		return "", err
	}
	coordinate := i.Version + "-" + version
	switch loader {
	case instance.LoaderForge:
		return forgeMaven + "net/minecraftforge/forge/" + coordinate + "/forge-" + coordinate + "-installer.jar", nil
	case instance.LoaderNeoForge:
		if i.Version == "1.20.1" {
			return neoMaven + "net/neoforged/forge/" + coordinate + "/forge-" + coordinate + "-installer.jar", nil
		}
		return neoMaven + "net/neoforged/neoforge/" + version + "/neoforge-" + version + "-installer.jar", nil
	default:
		return "", i18n.Error("forge.loader_required")
	}
}

func (g Runtime) forgeDir(i instance.Instance) string {
	_, version, _ := instance.NormalizeLoader(i.Version, i.Loader, i.LoaderVersion)
	return filepath.Join(g.Cache, strings.ToLower(i.Loader), i.Version, version, strings.ReplaceAll(platformKey(), "/", "-"))
}

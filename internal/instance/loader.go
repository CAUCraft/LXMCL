package instance

import (
	"regexp"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

const (
	LoaderVanilla  = "Vanilla"
	LoaderFabric   = "Fabric"
	LoaderForge    = "Forge"
	LoaderNeoForge = "NeoForge"
	LoaderQuilt    = "Quilt"
)

var forgeVersion = regexp.MustCompile(`^[0-9]{1,4}(\.[0-9]{1,5}){2,3}(-[a-zA-Z][a-zA-Z0-9]{0,15})?$`)

func (i Instance) Forge() bool         { return strings.EqualFold(i.Loader, LoaderForge) }
func (i Instance) NeoForge() bool      { return strings.EqualFold(i.Loader, LoaderNeoForge) }
func (i Instance) Quilt() bool         { return strings.EqualFold(i.Loader, LoaderQuilt) }
func (i Instance) UsesKnot() bool      { return i.Fabric() || i.Quilt() }
func (i Instance) UsesInstaller() bool { return i.Forge() || i.NeoForge() }

func (i Instance) LoaderName() string {
	if i.Vanilla() {
		return i18n.Text("loader.vanilla")
	}
	return strings.TrimSpace(i.Loader + " " + i.LoaderVersion)
}

func NormalizeLoader(game, kind, version string) (string, string, error) {
	if !SupportedRelease(game) {
		return "", "", i18n.Error("version.modern_required")
	}
	switch strings.ToLower(kind) {
	case "", "vanilla":
		if version == "" {
			return LoaderVanilla, "", nil
		}
	case "fabric", "fabric-loader":
		if ValidKnotVersion(version) {
			return LoaderFabric, version, nil
		}
	case "quilt", "quilt-loader":
		if ValidKnotVersion(version) {
			return LoaderQuilt, version, nil
		}
	case "forge":
		version = strings.TrimPrefix(version, game+"-")
		if forgeVersion.MatchString(version) {
			return LoaderForge, version, nil
		}
	case "neoforge":
		version = strings.TrimPrefix(version, game+"-")
		if forgeVersion.MatchString(version) {
			// NeoForge originally published its 1.20.1 builds as net.neoforged:forge.
			if game != "1.20.1" || strings.HasPrefix(version, "47.1.") {
				return LoaderNeoForge, version, nil
			}
		}
	default:
		return "", "", i18n.Error("loader.unsupported", kind)
	}
	return "", "", i18n.Error("loader.version_invalid", game, kind, version)
}

func (i Instance) Vanilla() bool { return i.Loader == "" || strings.EqualFold(i.Loader, "vanilla") }
func (i Instance) Fabric() bool  { return strings.EqualFold(i.Loader, "fabric") }

var fabricVersionID = regexp.MustCompile(`^[0-9][0-9A-Za-z.+_-]{0,63}$`)

func ValidKnotVersion(version string) bool { return fabricVersionID.MatchString(version) }

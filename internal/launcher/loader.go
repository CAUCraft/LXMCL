package launcher

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const (
	LoaderVanilla  = "Vanilla"
	LoaderFabric   = "Fabric"
	LoaderForge    = "Forge"
	LoaderNeoForge = "NeoForge"
	LoaderQuilt    = "Quilt"
)

var forgeVersion = regexp.MustCompile(`^[0-9]{1,4}(\.[0-9]{1,5}){2,3}(-[a-zA-Z][a-zA-Z0-9]{0,15})?$`)

func (i Instance) Forge() bool    { return strings.EqualFold(i.Loader, LoaderForge) }
func (i Instance) NeoForge() bool { return strings.EqualFold(i.Loader, LoaderNeoForge) }
func (i Instance) Quilt() bool    { return strings.EqualFold(i.Loader, LoaderQuilt) }
func (i Instance) knot() bool     { return i.Fabric() || i.Quilt() }
func (i Instance) patched() bool  { return i.Forge() || i.NeoForge() }

func (i Instance) LoaderName() string {
	if i.Vanilla() {
		return "原版"
	}
	return strings.TrimSpace(i.Loader + " " + i.LoaderVersion)
}

func normalizeLoader(game, kind, version string) (string, string, error) {
	if !modernRelease(game) {
		return "", "", errors.New("需要 Minecraft 1.20.1 或之后的正式版")
	}
	switch strings.ToLower(kind) {
	case "", "vanilla":
		if version == "" {
			return LoaderVanilla, "", nil
		}
	case "fabric", "fabric-loader":
		if fabricVersionID.MatchString(version) {
			return LoaderFabric, version, nil
		}
	case "quilt", "quilt-loader":
		if fabricVersionID.MatchString(version) {
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
		return "", "", fmt.Errorf("不支持加载器 %s", kind)
	}
	return "", "", fmt.Errorf("Minecraft %s 的 %s 加载器版本无效：%q", game, kind, version)
}

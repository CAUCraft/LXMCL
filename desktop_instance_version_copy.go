package main

import (
	"regexp"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/launcher"
)

// Loader metadata from older configurations may contain arbitrary text.
// Only accept a bounded version and a known release qualifier in shared text.
var shareableLoaderVersion = regexp.MustCompile(`^[0-9]{1,4}(\.[0-9]{1,5}){2,3}(-(alpha|beta|rc)(\.?[0-9]{1,4})?)?$`)

func instanceVersionClipboard(i launcher.Instance) string {
	game := i.Version
	if !instance.IsReleaseID(game) {
		game = "未知"
	}
	version := launcherVersion
	if _, err := releaseParts(version); err != nil {
		version = "未知"
	}
	return "LXMCL：" + version + "\nMinecraft：" + game + "\n加载器：" + shareableLoaderLabel(i)
}

func shareableLoaderLabel(i launcher.Instance) string {
	var kind string
	switch strings.ToLower(i.Loader) {
	case "", "vanilla":
		return "原版"
	case "fabric", "fabric-loader":
		kind = instance.LoaderFabric
	case "forge":
		kind = instance.LoaderForge
	case "neoforge":
		kind = instance.LoaderNeoForge
	case "quilt", "quilt-loader":
		kind = instance.LoaderQuilt
	default:
		return "未知"
	}
	_, version, err := instance.NormalizeLoader(i.Version, kind, i.LoaderVersion)
	if err != nil || !shareableLoaderVersion.MatchString(version) {
		return kind + "（版本未知）"
	}
	return kind + " " + version
}

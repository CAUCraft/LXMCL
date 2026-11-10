package catalog

import (
	"slices"
	"strings"
	"time"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/distribution"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/modpack"
)

type ModpackSource = modpack.Source

const (
	ModrinthModpacks   = modpack.Modrinth
	CurseForgeModpacks = modpack.CurseForge
)

type ModpackProject struct {
	ID, Name, Author, Summary, URL, IconURL string
	Source                                  ModpackSource
	distributionRestricted                  bool
}

type ModpackRelease struct {
	ID, Title, Filename, Channel string
	Games, Loaders               []string
	Published                    time.Time
	Size                         int64
	Unavailable                  string
	source                       ModpackSource
	asset                        distribution.Asset
	downloadBlocked              bool
}

type ModpackResults struct {
	Projects []ModpackProject
	Total    int
}

func (r ModpackRelease) Downloadable() bool {
	ext := ".mrpack"
	if r.source == CurseForgeModpacks {
		ext = ".zip"
	}
	return (r.source == ModrinthModpacks || r.source == CurseForgeModpacks) && !r.downloadBlocked &&
		content.ValidName(r.Filename, ext) && r.asset.Size > 0 && r.asset.Valid(modpack.MaxArchiveSize)
}

func (r ModpackRelease) Source() ModpackSource { return r.source }

// DownloadAsset enforces provider restrictions even if presentation fields change.
// Unsupported game/loader tags still permit saving the archive for use elsewhere.
func (r ModpackRelease) DownloadAsset() (distribution.Asset, bool) {
	if !r.Downloadable() {
		return distribution.Asset{}, false
	}
	a := r.asset
	a.URLs = slices.Clone(a.URLs)
	return a, true
}

func unsupportedPackTags(games, loaders []string) string {
	if slices.ContainsFunc(games, instance.IsReleaseID) && !slices.ContainsFunc(games, instance.SupportedRelease) {
		return i18n.Text("version.modern_required")
	}
	if len(loaders) > 0 && !slices.ContainsFunc(loaders, func(s string) bool {
		switch strings.ToLower(s) {
		case "fabric", "forge", "neoforge", "quilt", "minecraft", "vanilla":
			return true
		}
		return false
	}) {
		return i18n.Text("modpack.loader_unsupported")
	}
	return ""
}

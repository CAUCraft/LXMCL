package catalog

import (
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

const modrinthAPI = "https://api.modrinth.com/v2/"

const (
	fabricAPIProject  = "P7dR8mSH"
	quiltedAPIProject = "qvIfYCYJ"
)

var modrinthID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type PackProject struct {
	ID          string `json:"project_id"`
	Title       string `json:"title"`
	Author      string `json:"author"`
	Description string `json:"description"`
	Downloads   int    `json:"downloads"`
	IconURL     string `json:"icon_url"`
}

// ResourceFilter describes a catalog query independently of an installation.
type ResourceFilter struct{ Game, Loader string }

func (f ResourceFilter) valid(kind content.Kind) bool {
	return (f.Game == "" || instance.IsReleaseID(f.Game)) && modrinthType(kind) != "" &&
		(f.Loader == "" || kind == content.Mods && slices.Contains([]string{"fabric", "forge", "neoforge", "quilt"}, f.Loader))
}

func (f ResourceFilter) matches(v PackVersion) bool {
	return (f.Game == "" || slices.Contains(v.GameVersions, f.Game)) &&
		(f.Loader == "" || slices.Contains(v.Loaders, f.Loader))
}

func instanceResourceFilter(i instance.Instance, kind content.Kind) ResourceFilter {
	f := ResourceFilter{Game: i.Version}
	if kind == content.Mods {
		f.Loader = modrinthModLoader(i)
	}
	return f
}

func (p PackProject) PageURL(kind content.Kind) string {
	if !modrinthID.MatchString(p.ID) {
		return ""
	}
	if projectType := modrinthType(kind); projectType != "" {
		return "https://modrinth.com/" + projectType + "/" + p.ID
	}
	return ""
}

type PackVersion struct {
	ID           string    `json:"id"`
	ProjectID    string    `json:"project_id"`
	Number       string    `json:"version_number"`
	Type         string    `json:"version_type"`
	GameVersions []string  `json:"game_versions"`
	Loaders      []string  `json:"loaders"`
	Published    time.Time `json:"date_published"`
	Files        []PackFile
	Dependencies []ModDependency
}

type ModDependency struct {
	ProjectID string `json:"project_id"`
	VersionID string `json:"version_id"`
	Filename  string `json:"file_name"`
	Type      string `json:"dependency_type"`
}

type PackFile struct {
	URL, Filename string
	Size          int64
	Primary       bool
	Hashes        struct{ SHA512 string }
}

const PackPageSize = 20

type PackSearch struct {
	Hits  []PackProject
	Total int `json:"total_hits"`
}

func modrinthType(k content.Kind) string {
	switch k {
	case content.Mods:
		return "mod"
	case content.ResourcePacks:
		return "resourcepack"
	case content.ShaderPacks:
		return "shader"
	}
	return ""
}

func modrinthModLoader(i instance.Instance) string {
	if i.UsesKnot() || i.UsesInstaller() {
		return strings.ToLower(i.Loader)
	}
	return ""
}

// ValidModrinthID checks IDs and slugs before they are placed in API paths.
func ValidModrinthID(id string) bool { return modrinthID.MatchString(id) }

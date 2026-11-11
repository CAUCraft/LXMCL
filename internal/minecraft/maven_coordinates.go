package minecraft

import (
	"regexp"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

var mavenPart = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.+_-]{0,127}$`)

func mavenLibraryPath(name string) (string, error) {
	name, ext, hasExt := strings.Cut(name, "@")
	if hasExt && ext != "jar" {
		return "", i18n.Error("maven.extension_invalid", ext)
	}
	parts := strings.Split(name, ":")
	if len(parts) != 3 && len(parts) != 4 {
		return "", i18n.Error("maven.coordinates_invalid", name)
	}
	groups := strings.Split(parts[0], ".")
	for _, part := range append(groups, parts[1:]...) {
		if !mavenPart.MatchString(part) {
			return "", i18n.Error("maven.coordinates_invalid", name)
		}
	}
	return strings.Join(groups, "/") + "/" + parts[1] + "/" + parts[2] + "/" + strings.Join(parts[1:], "-") + ".jar", nil
}

func mavenLibraryKey(name string) string {
	name, _, _ = strings.Cut(name, "@")
	parts := strings.Split(name, ":")
	switch len(parts) {
	case 3:
		return parts[0] + ":" + parts[1]
	case 4:
		return parts[0] + ":" + parts[1] + ":" + parts[3]
	}
	return ""
}

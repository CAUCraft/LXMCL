package instance

import (
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

var (
	releaseID = regexp.MustCompile(`^[0-9]{1,3}\.[0-9]{1,3}(\.[0-9]{1,3})?$`)
	playerID  = regexp.MustCompile(`^[A-Za-z0-9_]{3,16}$`)
)

func ValidPlayerName(name string) bool { return playerID.MatchString(name) }

func IsReleaseID(id string) bool { return releaseID.MatchString(id) }

func SupportedRelease(id string) bool {
	if !releaseID.MatchString(id) {
		return false
	}
	var major, minor, patch int
	if n, _ := fmt.Sscanf(id, "%d.%d.%d", &major, &minor, &patch); n < 2 {
		return false
	}
	return major > 1 || (major == 1 && (minor > 20 || (minor == 20 && patch >= 1)))
}

func (i Instance) Validate() error {
	if !SupportedRelease(i.Version) || !filepath.IsAbs(i.GameDir) {
		return i18n.Error("instance.version_or_directory_invalid")
	}
	if _, _, err := NormalizeLoader(i.Version, i.Loader, i.LoaderVersion); err != nil {
		return err
	}
	return nil
}

func ValidateSettings(s Settings) error {
	if !filepath.IsAbs(s.GameRoot) {
		return i18n.Error("settings.game_root_invalid")
	}
	if !ValidPlayerName(s.Player) {
		return i18n.Error("account.offline_name_invalid")
	}
	return ValidateMemory(s.Memory)
}

func ValidateMemory(memory int) error {
	if memory < 1024 || memory > 65536 {
		return i18n.Error("launch.memory_invalid")
	}
	return nil
}

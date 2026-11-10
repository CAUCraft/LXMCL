package instance

import (
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

const (
	JavaInherit = ""
	JavaAuto    = "auto"
	JavaCustom  = "custom"

	DisplayDefault    = ""
	DisplayResolution = "resolution"
	DisplayFullscreen = "fullscreen"
)

// LaunchOptions overrides only the runtime options of one instance.
// Its zero value preserves the behavior of existing instances.
type LaunchOptions struct {
	JavaMode string `json:"java_mode,omitempty"`
	JavaPath string `json:"java_path,omitempty"`
	Memory   int    `json:"memory_mib,omitempty"`
	Display  string `json:"display,omitempty"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
}

func (o LaunchOptions) Validate() error {
	switch o.JavaMode {
	case JavaInherit, JavaAuto:
	case JavaCustom:
		if !filepath.IsAbs(o.JavaPath) {
			return i18n.Error("launch.java_path_required")
		}
	default:
		return i18n.Error("launch.java_mode_invalid")
	}
	if o.Memory != 0 {
		if err := ValidateMemory(o.Memory); err != nil {
			return err
		}
	}
	switch o.Display {
	case DisplayDefault, DisplayFullscreen:
	case DisplayResolution:
		if o.Width < 320 || o.Width > 8192 || o.Height < 240 || o.Height > 8192 {
			return i18n.Error("launch.resolution_invalid")
		}
	default:
		return i18n.Error("launch.display_mode_invalid")
	}
	return nil
}

func (o LaunchOptions) JavaExecutable(global string) string {
	switch o.JavaMode {
	case JavaAuto:
		return ""
	case JavaCustom:
		return o.JavaPath
	default:
		return global
	}
}

func (o LaunchOptions) MaxMemory(global int) int {
	if o.Memory != 0 {
		return o.Memory
	}
	return global
}

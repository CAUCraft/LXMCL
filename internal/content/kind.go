package content

import (
	"time"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

type Kind string

const (
	ResourcePacks Kind = "resourcepacks"
	Mods          Kind = "mods"
	ShaderPacks   Kind = "shaderpacks"
	MaxSize            = 512 << 20
)

type File struct {
	Name             string
	ModTitle, ModIDs string
	mods             []manifestMod
	location         *modFileLocation
	Size             int64
	Directory        bool
	Enabled          bool
	Modified         time.Time
}

func (f File) DiskName() string {
	if f.Enabled {
		return f.Name
	}
	return f.Name + ".disabled"
}

func (k Kind) Format(i instance.Instance) (ext string, markers []string, err error) {
	switch k {
	case ResourcePacks:
		return ".zip", []string{"pack.mcmeta"}, nil
	case Mods:
		switch {
		case i.Fabric():
			return ".jar", []string{"fabric.mod.json"}, nil
		case i.Forge():
			return ".jar", []string{"META-INF/mods.toml"}, nil
		case i.NeoForge():
			switch i.Version {
			case "1.20.1", "1.20.2", "1.20.3", "1.20.4":
				return ".jar", []string{"META-INF/mods.toml"}, nil
			default:
				return ".jar", []string{"META-INF/neoforge.mods.toml"}, nil
			}
		case i.Quilt():
			return ".jar", []string{"quilt.mod.json", "fabric.mod.json"}, nil
		}
	case ShaderPacks:
		if i.UsesKnot() || i.UsesInstaller() {
			return ".zip", []string{"shaders/"}, nil
		}
	}
	return "", nil, i18n.Error("resource.kind_unsupported")
}

func (k Kind) Supports(i instance.Instance) bool {
	_, _, err := k.Format(i)
	return err == nil && i.Validate() == nil
}

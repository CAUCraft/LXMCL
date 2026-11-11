package minecraft

import (
	"archive/zip"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

var javaClass = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$.]{0,255}$`)

type forgeLibrary struct {
	Name      string
	Rules     []rule
	Downloads struct{ Artifact artifact }
}

type forgeProfile struct {
	Platform, ID, InheritsFrom, MainClass, LoaderVersion string
	Arguments                                            loaderArguments
	Libraries                                            []forgeLibrary
	Auxiliary                                            []forgeLibrary `json:",omitempty"`
}

type forgeInstaller struct {
	Minecraft, Version, JSON string
	Libraries                []forgeLibrary
	Data                     map[string]struct{ Client string }
	Processors               []struct {
		Sides   []string
		Outputs map[string]string
	}
}

func forgeMetadata(path string) (forgeInstaller, forgeProfile, error) {
	var installer forgeInstaller
	var profile forgeProfile
	z, err := zip.OpenReader(path)
	if err != nil {
		return installer, profile, err
	}
	defer z.Close()
	entry := func(name string) *zip.File {
		for _, f := range z.File {
			if f.Name == name && f.Mode().IsRegular() {
				return f
			}
		}
		return nil
	}
	if err := storage.ReadArchiveJSON(entry("install_profile.json"), maxMetadata, &installer); err != nil {
		return installer, profile, i18n.Error("forge.manifest_read_failed", err)
	}
	name := strings.TrimPrefix(installer.JSON, "/")
	if !storage.ValidPortablePath(name) || filepath.Ext(name) != ".json" {
		return installer, profile, i18n.Error("forge.client_profile_unsupported")
	}
	if err := storage.ReadArchiveJSON(entry(name), maxMetadata, &profile); err != nil {
		return installer, profile, i18n.Error("forge.client_profile_read_failed", err)
	}
	return installer, profile, nil
}

func validateForgeProfile(i instance.Instance, p forgeProfile) error {
	if p.InheritsFrom != i.Version || !storage.ValidPortablePath(p.ID) || strings.Contains(p.ID, "/") ||
		!javaClass.MatchString(p.MainClass) || len(p.Libraries) == 0 {
		return i18n.Error("forge.client_profile_mismatch", i.LoaderName(), i.Version)
	}
	return nil
}

func (p forgeInstaller) clientLibraries() ([]forgeLibrary, error) {
	value := func(raw string) string {
		if strings.HasPrefix(raw, "{") && strings.HasSuffix(raw, "}") {
			return p.Data[raw[1:len(raw)-1]].Client
		}
		return raw
	}
	generated := make(map[string]string)
	add := func(raw, sha string) error {
		coord := strings.TrimSuffix(strings.TrimPrefix(raw, "["), "]")
		if "["+coord+"]" != raw || (strings.Contains(coord, "@") && !strings.HasSuffix(coord, "@jar")) {
			return nil
		}
		path, err := mavenLibraryPath(coord)
		if err != nil {
			return err
		}
		if sha != "" {
			sha = strings.Trim(sha, "'")
			if _, err := (artifact{SHA: sha, Size: unknownSize}).Checksum(); err != nil {
				return i18n.Error("forge.output_checksum_invalid", path, err)
			}
		}
		if old := generated[path]; old != "" {
			if sha == "" || strings.EqualFold(old, sha) {
				return nil
			}
			return i18n.Error("forge.output_checksum_conflict", path)
		}
		generated[path] = sha
		return nil
	}
	for _, step := range p.Processors {
		if len(step.Sides) != 0 && !slices.Contains(step.Sides, "client") {
			continue
		}
		for dest, sha := range step.Outputs {
			if err := add(value(dest), value(sha)); err != nil {
				return nil, err
			}
		}
	}
	for _, key := range []string{"PATCHED", "MC_EXTRA"} {
		if err := add(p.Data[key].Client, ""); err != nil {
			return nil, err
		}
	}
	out := make([]forgeLibrary, 0, len(generated)+len(p.Libraries))
	for path, sha := range generated {
		var lib forgeLibrary
		lib.Downloads.Artifact = artifact{Path: path, SHA: sha}
		out = append(out, lib)
	}
	for _, lib := range p.Libraries {
		a := lib.Downloads.Artifact
		if a.URL != "" && strings.HasSuffix(a.Path, ".jar") {
			out = append(out, lib)
		}
	}
	return out, nil
}

func (g Runtime) readForge(i instance.Instance) (forgeProfile, error) {
	var p forgeProfile
	ok, err := storage.ReadJSON(filepath.Join(g.forgeDir(i), "verified.json"), &p)
	if err != nil {
		return p, err
	}
	if !ok || p.Platform != platformKey() || p.LoaderVersion != i.LoaderVersion {
		return p, i18n.Error("loader.files_unverified", i.LoaderName())
	}
	return p, validateForgeProfile(i, p)
}

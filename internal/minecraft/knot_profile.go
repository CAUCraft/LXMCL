package minecraft

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

const fabricMeta = "https://meta.fabricmc.net/v2/versions/loader/"
const quiltMeta = "https://meta.quiltmc.org/v3/versions/loader/"

type loaderArguments struct{ JVM, Game []json.RawMessage }

type knotRuntime struct {
	GameVersion   string
	LoaderVersion string
	MainClass     string
	Arguments     loaderArguments
	Libraries     []artifact
}

func (g Runtime) knotDir(i instance.Instance) string {
	loader := "fabric"
	if i.Quilt() {
		loader = "quilt"
	}
	return filepath.Join(g.Cache, loader, i.Version, i.LoaderVersion)
}

func knotSource(i instance.Instance) (meta, prefix, main string) {
	if i.Quilt() {
		return quiltMeta, "quilt-loader-", "org.quiltmc.loader.impl.launch.knot.KnotClient"
	}
	return fabricMeta, "fabric-loader-", "net.fabricmc.loader.impl.launch.knot.KnotClient"
}

func knotName(i instance.Instance) string {
	if i.Quilt() {
		return instance.LoaderQuilt
	}
	return instance.LoaderFabric
}

func (g Runtime) readKnot(i instance.Instance) (knotRuntime, error) {
	_, _, main := knotSource(i)
	var profile knotRuntime
	ok, err := storage.ReadJSON(filepath.Join(g.knotDir(i), "verified.json"), &profile)
	if err != nil {
		return profile, err
	}
	if !ok || profile.GameVersion != i.Version || profile.LoaderVersion != i.LoaderVersion ||
		profile.MainClass != main || len(profile.Libraries) == 0 {
		return profile, i18n.Error("loader.files_unverified", knotName(i))
	}
	return profile, nil
}

func knotLibraryPath(name string) (string, error) {
	parts := strings.Split(name, ":")
	if len(parts) != 3 || !mavenPart.MatchString(parts[1]) || !mavenPart.MatchString(parts[2]) {
		return "", i18n.Error("loader.library_unsupported", name)
	}
	group := strings.Split(parts[0], ".")
	for _, part := range group {
		if !mavenPart.MatchString(part) {
			return "", i18n.Error("loader.library_coordinates_invalid", name)
		}
	}
	return strings.Join(group, "/") + "/" + parts[1] + "/" + parts[2] + "/" + parts[1] + "-" + parts[2] + ".jar", nil
}

func (g Runtime) knotFiles(i instance.Instance, libs []artifact) ([]fileDownload, []string, error) {
	var files []fileDownload
	var classpath []string
	for _, lib := range libs {
		if !filepath.IsLocal(lib.Path) || strings.ContainsAny(lib.Path, ":\\") || filepath.Ext(lib.Path) != ".jar" {
			return nil, nil, i18n.Error("loader.library_path_invalid", knotName(i), lib.Path)
		}
		path := filepath.Join(g.knotDir(i), "libraries", filepath.FromSlash(lib.Path))
		files = append(files, fileDownload{path, lib})
		classpath = append(classpath, path)
	}
	return files, classpath, nil
}

func (g Runtime) verifyKnot(ctx context.Context, i instance.Instance) (knotRuntime, []string, error) {
	p, err := g.readKnot(i)
	if err != nil {
		return p, nil, err
	}
	files, cp, err := g.knotFiles(i, p.Libraries)
	if err == nil {
		err = transfer.Batch(ctx, files, func(ctx context.Context, f fileDownload) error { return verifyArtifact(ctx, f) }, nil)
	}
	return p, cp, err
}

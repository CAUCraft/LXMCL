package launcher

import (
	"context"
	"encoding/json"
	"io"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

const fabricMeta = "https://meta.fabricmc.net/v2/versions/loader/"
const quiltMeta = "https://meta.quiltmc.org/v3/versions/loader/"

var (
	mavenPart = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.+_-]{0,127}$`)
)

func downloadJSON(ctx context.Context, d *downloads, source string, out any) error {
	return retry(ctx, func() error {
		res, err := d.get(ctx, source)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		body, err := io.ReadAll(io.LimitReader(res.Body, maxMetadata+1))
		if err != nil {
			return err
		}
		if len(body) > maxMetadata {
			return i18n.Error("metadata.response_too_large")
		}
		return json.Unmarshal(body, out)
	})
}

func FabricVersions(ctx context.Context, gameVersion string) ([]string, error) {
	if !instance.SupportedRelease(gameVersion) {
		return nil, i18n.Error("version.select_supported")
	}
	var entries []struct {
		Loader struct {
			Version string
			Stable  bool
		}
	}
	if err := downloadJSON(ctx, &downloads{http: officialHTTP}, fabricMeta+url.PathEscape(gameVersion), &entries); err != nil {
		return nil, err
	}
	var stable []string
	for _, entry := range entries {
		if entry.Loader.Stable && instance.ValidKnotVersion(entry.Loader.Version) {
			stable = append(stable, entry.Loader.Version)
		}
	}
	return stable, nil
}

type loaderArguments struct{ JVM, Game []json.RawMessage }

type knotRuntime struct {
	GameVersion   string
	LoaderVersion string
	MainClass     string
	Arguments     loaderArguments
	Libraries     []artifact
}

func (g Game) knotDir(i Instance) string {
	loader := "fabric"
	if i.Quilt() {
		loader = "quilt"
	}
	return filepath.Join(g.Cache, loader, i.Version, i.LoaderVersion)
}

func knotSource(i Instance) (meta, prefix, main string) {
	if i.Quilt() {
		return quiltMeta, "quilt-loader-", "org.quiltmc.loader.impl.launch.knot.KnotClient"
	}
	return fabricMeta, "fabric-loader-", "net.fabricmc.loader.impl.launch.knot.KnotClient"
}

func knotName(i Instance) string {
	if i.Quilt() {
		return LoaderQuilt
	}
	return LoaderFabric
}

func (g Game) readKnot(i Instance) (knotRuntime, error) {
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

func (g Game) knotFiles(i Instance, libs []artifact) ([]fileDownload, []string, error) {
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

func (g Game) installKnot(ctx context.Context, i Instance, d *downloads, report func(Progress)) error {
	loader := knotName(i)
	if _, _, err := g.verifyKnot(ctx, i); err == nil {
		return nil
	}
	report(Progress{Stage: i18n.Text("progress.fetch_loader", loader)})
	var remote struct {
		ID           string
		InheritsFrom string
		MainClass    string
		Arguments    loaderArguments
		Libraries    []struct {
			Name, URL, SHA1 string
			Size            int64
		}
	}
	meta, prefix, main := knotSource(i)
	source := meta + url.PathEscape(i.Version) + "/" + url.PathEscape(i.LoaderVersion) + "/profile/json"
	if err := downloadJSON(ctx, d, source, &remote); err != nil {
		return err
	}
	if remote.InheritsFrom != i.Version || remote.ID != prefix+i.LoaderVersion+"-"+i.Version ||
		remote.MainClass != main || len(remote.Libraries) == 0 {
		return i18n.Error("loader.profile_mismatch", loader)
	}
	p := knotRuntime{GameVersion: i.Version, LoaderVersion: i.LoaderVersion, MainClass: remote.MainClass, Arguments: remote.Arguments}
	var checksums []int
	for _, lib := range remote.Libraries {
		path, err := knotLibraryPath(lib.Name)
		if err != nil {
			return err
		}
		if lib.URL != fabricMaven && lib.URL != "https://libraries.minecraft.net/" &&
			!(i.Quilt() && lib.URL == "https://maven.quiltmc.org/repository/release/") {
			return i18n.Error("loader.library_source_invalid", loader, lib.URL)
		}
		a := artifact{Path: path, URL: lib.URL + path, SHA: lib.SHA1, Size: lib.Size}
		if a.Size == 0 {
			a.Size = unknownSize
		}
		if a.SHA == "" {
			checksums = append(checksums, len(p.Libraries))
		} else if _, err := a.Checksum(); err != nil {
			return err
		}
		p.Libraries = append(p.Libraries, a)
	}
	if err := transfer.Batch(ctx, checksums, func(ctx context.Context, index int) error {
		lib := &p.Libraries[index]
		sha, err := d.fetchSHA1(ctx, lib.URL)
		if err != nil {
			return i18n.Error("loader.library_checksum_failed", filepath.Base(lib.Path), err)
		}
		lib.SHA = sha
		_, err = lib.Checksum()
		return err
	}, nil); err != nil {
		return err
	}
	files, _, err := g.knotFiles(i, p.Libraries)
	if err != nil {
		return err
	}
	if err := d.batch(ctx, i18n.Text("progress.download_and_verify", loader), files, report); err != nil {
		return err
	}
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return storage.WriteFile(filepath.Join(g.knotDir(i), "verified.json"), data)
}

func (g Game) verifyKnot(ctx context.Context, i Instance) (knotRuntime, []string, error) {
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

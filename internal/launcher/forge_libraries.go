package launcher

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

func (g Game) prefetchForgeLibraries(ctx context.Context, d *downloads, game string, installer, client []forgeLibrary, report func(Progress)) error {
	seen := make(map[string]artifact)
	var files []fileDownload
	for _, lib := range append(slices.Clone(installer), client...) {
		a := lib.Downloads.Artifact
		if a.URL == "" {
			continue // embedded in the installer or produced by a client processor
		}
		u, err := url.Parse(a.URL)
		_, neoAsset := neoMavenPath(a.URL)
		if err != nil || (!officialURL(u) && !neoAsset) || !storage.ValidPortablePath(a.Path) || a.Size < 0 || a.Size > maxForgeLibrarySize {
			return i18n.Error("forge.library_invalid", lib.Name)
		}
		if a.Size == 0 {
			a.Size = unknownSize
		}
		if prev, exists := seen[a.Path]; exists {
			if !strings.EqualFold(prev.SHA, a.SHA) || prev.Size != a.Size {
				return i18n.Error("forge.library_checksum_conflict", a.Path)
			}
			continue
		}
		seen[a.Path] = a
		files = append(files, fileDownload{filepath.Join(game, "libraries", filepath.FromSlash(a.Path)), a})
	}
	stage := i18n.Text("progress.download_loader_libraries")
	report(Progress{Stage: d.mirrorStage(stage), Total: len(files)})
	return transfer.Batch(ctx, files, func(ctx context.Context, f fileDownload) error {
		sum, err := f.file.Checksum()
		if err != nil {
			return i18n.Error("common.context_error", f.file.Path, err)
		}
		cache := filepath.Join(g.Cache, "loader-libraries", hex.EncodeToString(sum))
		base := filepath.Join(g.Cache, "libraries", filepath.FromSlash(f.file.Path))
		if ok, err := transfer.Check(ctx, base, f.file, sum, 128<<20); ok && err == nil {
			cache = base
		} else {
			if err := d.fileBound(ctx, cache, f.file, 128<<20); err != nil {
				return i18n.Error("common.context_error", f.file.Path, err)
			}
		}
		if err := os.MkdirAll(filepath.Dir(f.path), 0755); err != nil {
			return err
		}
		return copyForgeFile(ctx, f.path, cache)
	}, func(done int) { report(Progress{Stage: d.mirrorStage(stage), Done: done, Total: len(files)}) })
}

func copyForgeFile(ctx context.Context, dest, source string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, transfer.Reader(ctx, in))
	return errors.Join(err, out.Close())
}

func forgeLibraryPath(lib forgeLibrary) (string, error) {
	if lib.Downloads.Artifact.Path != "" {
		if p := lib.Downloads.Artifact.Path; storage.ValidPortablePath(p) && filepath.Ext(p) == ".jar" {
			return p, nil
		}
		return "", i18n.Error("forge.library_path_invalid", lib.Downloads.Artifact.Path)
	}
	return mavenLibraryPath(lib.Name)
}

func attestForgeLibraries(ctx context.Context, root string, libs []forgeLibrary) ([]forgeLibrary, error) {
	result := make([]forgeLibrary, 0, len(libs))
	seen := make(map[string]bool)
	for _, lib := range libs {
		if !allowed(lib.Rules, nil) {
			continue
		}
		path, err := forgeLibraryPath(lib)
		if err != nil {
			return nil, err
		}
		if seen[path] {
			continue
		}
		seen[path] = true
		full := filepath.Join(root, filepath.FromSlash(path))
		st, err := os.Lstat(full)
		if err != nil || !st.Mode().IsRegular() || st.Size() <= 0 || st.Size() > maxForgeLibrarySize {
			return nil, i18n.Error("forge.library_not_generated", path)
		}
		a := lib.Downloads.Artifact
		if a.SHA != "" {
			if a.Size == 0 {
				a.Size = st.Size()
			}
			if err := verifyArtifact(ctx, fileDownload{full, a}); err != nil {
				return nil, err
			}
		} else {
			// Some processor outputs don't have a hash in version.json. The
			// official installer checks its own outputs before reporting success.
			f, err := os.Open(full)
			if err != nil {
				return nil, err
			}
			h := sha1.New()
			n, err := io.Copy(h, io.LimitReader(transfer.Reader(ctx, f), st.Size()+1))
			f.Close()
			if err != nil || n != st.Size() {
				return nil, i18n.Error("forge.output_verify_failed", path, errors.Join(err, io.ErrUnexpectedEOF))
			}
			a.SHA, a.Size = hex.EncodeToString(h.Sum(nil)), st.Size()
		}
		a.Path = path
		lib.Downloads.Artifact = a
		result = append(result, lib)
	}
	return result, nil
}

// Forge discovers the patched client and some libraries through libraryDirectory
// rather than through the version profile's classpath.

func (g Game) verifyForge(ctx context.Context, i Instance, p forgeProfile) ([]string, error) {
	root := filepath.Join(g.forgeDir(i), "libraries")
	var files []fileDownload
	var cp []string
	for n, lib := range append(slices.Clone(p.Libraries), p.Auxiliary...) {
		path, err := forgeLibraryPath(lib)
		if err != nil {
			return nil, err
		}
		full := filepath.Join(root, filepath.FromSlash(path))
		files = append(files, fileDownload{full, lib.Downloads.Artifact})
		if n < len(p.Libraries) {
			cp = append(cp, full)
		}
	}
	return cp, transfer.Batch(ctx, files, verifyArtifact, nil)
}

func (g Game) mergeForgeClasspath(m metadata, p forgeProfile, base, loader []string) []string {
	replaced := make(map[string]bool)
	for _, lib := range p.Libraries {
		if k := mavenLibraryKey(lib.Name); k != "" {
			replaced[k] = true
		}
	}
	skip := make(map[string]bool)
	for _, lib := range m.Libraries {
		if replaced[mavenLibraryKey(lib.Name)] {
			skip[filepath.Join(g.Cache, "libraries", filepath.FromSlash(lib.Downloads.Artifact.Path))] = true
		}
	}
	return append(loader, slices.DeleteFunc(base, func(path string) bool { return skip[path] })...)
}

func ignoreCachedVanillaJar(jvm []string, version string) {
	const flag = "-DignoreList="
	for n, arg := range jvm {
		if list, ok := strings.CutPrefix(arg, flag); ok {
			// Forge's inherited profile names the vanilla jar like Mojang's
			// launcher does. LXMCL stores that same verified artifact as client.jar.
			names := strings.Split(list, ",")
			if slices.Contains(names, version+".jar") && !slices.Contains(names, "client.jar") {
				jvm[n] = arg + ",client.jar"
			}
		}
	}
}

const maxForgeLibrarySize = 1 << 30

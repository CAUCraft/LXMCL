package launcher

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	launcherlog "github.com/CAUCraft/LXMCL/internal/log"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

const (
	forgeMaven = "https://maven.minecraftforge.net/"
	neoMaven   = "https://maven.neoforged.net/releases/"
)

func forgeInstallerURL(i Instance) (string, error) {
	loader, version, err := instance.NormalizeLoader(i.Version, i.Loader, i.LoaderVersion)
	if err != nil {
		return "", err
	}
	coordinate := i.Version + "-" + version
	switch loader {
	case LoaderForge:
		return forgeMaven + "net/minecraftforge/forge/" + coordinate + "/forge-" + coordinate + "-installer.jar", nil
	case LoaderNeoForge:
		if i.Version == "1.20.1" {
			return neoMaven + "net/neoforged/forge/" + coordinate + "/forge-" + coordinate + "-installer.jar", nil
		}
		return neoMaven + "net/neoforged/neoforge/" + version + "/neoforge-" + version + "-installer.jar", nil
	default:
		return "", i18n.Error("forge.loader_required")
	}
}

func (g Game) forgeDir(i Instance) string {
	_, version, _ := instance.NormalizeLoader(i.Version, i.Loader, i.LoaderVersion)
	return filepath.Join(g.Cache, strings.ToLower(i.Loader), i.Version, version, strings.ReplaceAll(platformKey(), "/", "-"))
}

// The installer's download runs from an empty Mojang-style directory. It never
// receives the modpack directory, launcher credentials, or the user's profiles.

func (g Game) installForge(ctx context.Context, i Instance, m metadata, javaPath string, d *downloads, report func(Progress)) error {
	dest := g.forgeDir(i)
	if p, err := g.readForge(i); err == nil {
		if _, err := g.verifyForge(ctx, i, p); err == nil {
			return nil
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	report(Progress{Stage: i18n.Text("progress.check_installer_java")})
	j, err := FindJava(ctx, i.Launch.JavaExecutable(javaPath), m.JavaVersion.MajorVersion)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return i18n.Error("forge.java_required", ErrJavaRequired, i.LoaderName(), m.JavaVersion.MajorVersion, err)
	}
	j.Path = javaConsole(j.Path)
	src, err := forgeInstallerURL(i)
	if err != nil {
		return err
	}
	report(Progress{Stage: i18n.Text("progress.download_installer", i.Loader)})
	sha, err := d.fetchSHA1(ctx, src)
	if err != nil {
		return i18n.Error("forge.installer_checksum_failed", i.LoaderName(), err)
	}
	// A modern installer can embed large binary patches.
	installerJar := filepath.Join(g.Cache, "installers", sha+".jar")
	if err := d.fileBound(ctx, installerJar, artifact{URL: src, SHA: sha, Size: unknownSize}, 128<<20); err != nil {
		return i18n.Error("forge.installer_failed", i.LoaderName(), err)
	}
	installer, p, err := forgeMetadata(installerJar)
	if err != nil {
		return err
	}
	if err := validateForgeProfile(i, p); err != nil {
		return err
	}
	if installer.Minecraft != i.Version || installer.Version != p.ID {
		return i18n.Error("forge.installer_version_mismatch", i.LoaderName())
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(dest), ".installer-*")
	if err != nil {
		return err
	}
	keepStage := false
	defer func() {
		if !keepStage {
			os.RemoveAll(stage)
		}
	}()
	game := filepath.Join(stage, "install")
	base := filepath.Join(game, "versions", i.Version)
	if err := os.MkdirAll(base, 0755); err != nil {
		return err
	}
	if err := storage.WriteFile(filepath.Join(game, "launcher_profiles.json"), []byte(`{"profiles":{}}`)); err != nil {
		return err
	}
	for name, target := range map[string]string{"client.jar": i.Version + ".jar", "version.json": i.Version + ".json"} {
		if err := copyForgeFile(ctx, filepath.Join(base, target), filepath.Join(g.versionDir(i.Version), name)); err != nil {
			return err
		}
	}
	if err := g.prefetchForgeLibraries(ctx, d, game, installer.Libraries, p.Libraries, report); err != nil {
		return err
	}
	report(Progress{Stage: i18n.Text("progress.run_installer", i.Loader)})
	if err := g.runForgeInstaller(ctx, j.Path, installerJar, game, i); err != nil {
		return err
	}
	var installed forgeProfile
	ok, err := storage.ReadJSON(filepath.Join(game, "versions", p.ID, p.ID+".json"), &installed)
	if err != nil || !ok || installed.ID != p.ID || validateForgeProfile(i, installed) != nil {
		return i18n.Error("forge.installer_profile_missing", i.LoaderName())
	}
	// The version JSON itself is taken from the verified installer. Preserve
	// only the launch libraries from the staging area, including generated jars.
	report(Progress{Stage: i18n.Text("progress.verify_loader", i.Loader)})
	p.Platform, p.LoaderVersion = platformKey(), i.LoaderVersion
	libraryDir := filepath.Join(game, "libraries")
	p.Libraries, err = attestForgeLibraries(ctx, libraryDir, p.Libraries)
	if err != nil {
		return err
	}
	extra, err := installer.clientLibraries()
	if err != nil {
		return err
	}
	p.Auxiliary, err = attestForgeLibraries(ctx, libraryDir, extra)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if err := storage.WriteFile(filepath.Join(game, "verified.json"), encoded); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Move only our own cache entry out of the way while publishing its replacement.
	old := filepath.Join(stage, "old")
	if err := renameForgeCache(ctx, dest, old); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := renameForgeCache(ctx, game, dest); err != nil {
		if restore := renameForgeCache(context.WithoutCancel(ctx), old, dest); restore != nil && !errors.Is(restore, os.ErrNotExist) {
			keepStage = true
			return errors.Join(err, restore)
		}
		return err
	}
	return nil
}

func renameForgeCache(ctx context.Context, from, to string) error {
	var err error
	for attempt := range 6 {
		if err := ctx.Err(); err != nil {
			return err
		}
		err = os.Rename(from, to)
		if err == nil || runtime.GOOS != "windows" || attempt == 5 ||
			(!errors.Is(err, os.ErrPermission) && !errors.Is(err, syscall.Errno(32))) {
			return err
		}
		// Windows scanners may briefly hold newly installed JARs open.
		if err := waitContext(ctx, time.Duration(1<<attempt)*50*time.Millisecond); err != nil {
			return err
		}
	}
	return err
}

func (g Game) runForgeInstaller(ctx context.Context, java, installer, game string, i Instance) error {
	log, err := launcherlog.CreateInstaller(g.Cache, i.Loader)
	if err != nil {
		return err
	}
	defer log.Close()
	ctx, stop := context.WithTimeout(ctx, 15*time.Minute)
	defer stop()
	cmd := exec.CommandContext(ctx, java, "-Djava.awt.headless=true", "-Duser.home="+game,
		"-jar", installer, "--installClient", game)
	hideConsole(cmd)
	cmd.Dir, cmd.Stdout, cmd.Stderr = game, log, log
	// Do not pass key/token environment variables or inherited Java agents.
	cmd.Env = javaEnvironment(true)
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return context.Canceled
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return i18n.Error("forge.installer_timed_out", i.LoaderName(), log.Name(), ctx.Err())
		}
		return i18n.Error("forge.installer_process_failed", i.LoaderName(), err, log.Name())
	}
	return nil
}

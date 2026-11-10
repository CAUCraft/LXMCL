package launcher

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/javaruntime"
	launcherlog "github.com/CAUCraft/LXMCL/internal/log"
	"github.com/CAUCraft/LXMCL/internal/process"
)

type Running struct {
	PID  int
	Log  string
	Done <-chan error
}

func (g Game) Start(ctx context.Context, i Instance, settings Settings) (Running, error) {
	return g.StartOnServer(ctx, i, settings, "")
}

func (g Game) StartOnServer(ctx context.Context, i Instance, settings Settings, server string) (Running, error) {
	return g.StartWithSession(ctx, i, settings, server, nil)
}

func (g Game) StartWithSession(ctx context.Context, i Instance, settings Settings, server string, session *MinecraftSession) (Running, error) {
	identity, err := launchIdentity(settings.Player, session)
	if err != nil {
		return Running{}, err
	}
	return g.start(ctx, i, settings, server, identity, nil)
}

func (g Game) StartWithStation(ctx context.Context, i Instance, settings Settings, server string, session StationSession) (Running, error) {
	identity, err := stationIdentity(session)
	if err != nil {
		return Running{}, err
	}
	return g.start(ctx, i, settings, server, identity, &session)
}

func stationIdentity(session StationSession) (gameIdentity, error) {
	p := session.Profile
	if !session.validFor(p) {
		return gameIdentity{}, ErrSkinStationLogin
	}
	return gameIdentity{name: p.Name, uuid: p.ID, token: session.access, kind: "mojang"}, nil
}

func (g Game) start(ctx context.Context, i Instance, settings Settings, server string, identity gameIdentity, station *StationSession) (Running, error) {
	return g.startProgress(ctx, i, settings, server, identity, station, nil)
}

func (g Game) startProgress(ctx context.Context, i Instance, settings Settings, server string, identity gameIdentity, station *StationSession, report func(Progress)) (Running, error) {
	if err := i.Validate(); err != nil {
		return Running{}, err
	}
	if err := i.Launch.Validate(); err != nil {
		return Running{}, err
	}
	memory := i.Launch.MaxMemory(settings.Memory)
	if err := instance.ValidateMemory(memory); err != nil {
		return Running{}, err
	}
	m, cp, err := g.verifyInstalled(ctx, i)
	libraryDir, launchVersion := filepath.Join(g.Cache, "libraries"), m.ID
	if err == nil && i.UsesKnot() {
		var profile knotRuntime
		var libs []string
		profile, libs, err = g.verifyKnot(ctx, i)
		if err == nil {
			cp = append(libs, cp...)
			m.MainClass = profile.MainClass
			m.Arguments.JVM = append(m.Arguments.JVM, profile.Arguments.JVM...)
			m.Arguments.Game = append(m.Arguments.Game, profile.Arguments.Game...)
		}
	}
	if err == nil && i.UsesInstaller() {
		var profile forgeProfile
		profile, err = g.readForge(i)
		if err == nil {
			var libs []string
			libs, err = g.verifyForge(ctx, i, profile)
			if err == nil {
				cp = g.mergeForgeClasspath(m, profile, cp, libs)
				m.MainClass = profile.MainClass
				m.Arguments.JVM = append(m.Arguments.JVM, profile.Arguments.JVM...)
				m.Arguments.Game = append(m.Arguments.Game, profile.Arguments.Game...)
				launchVersion = profile.ID
				libraryDir = filepath.Join(g.forgeDir(i), "libraries")
			}
		}
	}
	if err != nil {
		if ctx.Err() != nil {
			return Running{}, ctx.Err()
		}
		return Running{}, i18n.Error("common.wrapped_errors", ErrRepairRequired, err)
	}
	for _, path := range cp {
		if strings.ContainsRune(path, os.PathListSeparator) {
			return Running{}, i18n.Error("launch.cache_path_separator", os.PathListSeparator)
		}
	}
	j, err := g.resolveJava(ctx, i.Launch.JavaExecutable(settings.JavaPath), m, report)
	if err != nil {
		if ctx.Err() != nil {
			return Running{}, ctx.Err()
		}
		return Running{}, i18n.Error("common.wrapped_errors", ErrJavaRequired, err)
	}
	if err := ctx.Err(); err != nil {
		return Running{}, err
	}
	natives := filepath.Join(i.GameDir, ".lxmcl-natives")
	if err := os.MkdirAll(natives, 0755); err != nil {
		return Running{}, i18n.Error("launch.runtime_dir_create_failed", err)
	}
	values := map[string]string{
		"natives_directory": natives, "launcher_name": "LXMCL", "launcher_version": "0.2",
		"classpath": strings.Join(cp, string(os.PathListSeparator)), "classpath_separator": string(os.PathListSeparator),
		"library_directory": libraryDir, "auth_player_name": identity.name,
		"version_name": launchVersion, "game_directory": i.GameDir, "assets_root": filepath.Join(g.Cache, "assets"),
		"assets_index_name": m.AssetIndex.ID, "auth_uuid": identity.uuid, "auth_access_token": identity.token,
		"clientid": identity.clientID, "auth_xuid": identity.xuid, "user_type": identity.kind, "version_type": m.Type,
		"user_properties": "{}",
	}
	jvm, err := gameArgs(m.Arguments.JVM, values, nil)
	if err != nil {
		return Running{}, i18n.Error("launch.jvm_args_failed", err)
	}
	if i.UsesInstaller() {
		ignoreCachedVanillaJar(jvm, launchVersion)
	}
	game, err := clientArgs(m, values, i.Launch, server)
	if err != nil {
		return Running{}, i18n.Error("launch.client_args_failed", err)
	}
	args := append([]string{"-Xmx" + strconv.Itoa(memory) + "M"}, jvm...)
	if station != nil {
		extra, err := station.launchArgs(ctx, g.Cache)
		if err != nil {
			return Running{}, err
		}
		args = append(args, extra...)
	}
	if l := m.Logging.Client; l.Argument != "" {
		args = append(args, strings.ReplaceAll(l.Argument, "${path}", filepath.Join(g.Cache, "assets", "log_configs", l.File.ID)))
	}
	args = append(append(args, m.MainClass), game...)
	log, err := launcherlog.CreateGame(i.GameDir, launcherlog.GameHeader{
		Version: i.Version, JavaPath: j.Path, JavaMajor: j.Major, MemoryMiB: memory,
	})
	if err != nil {
		return Running{}, err
	}
	// The game process owns its own lifetime after Start, independent of the launcher window.
	cmd := exec.Command(j.Path, args...)
	process.HideConsole(cmd)
	cmd.Dir, cmd.Stdout, cmd.Stderr = i.GameDir, log, log
	cmd.Env = javaruntime.Environment(false)
	if err := ctx.Err(); err != nil {
		log.Close()
		return Running{}, err
	}
	if err := cmd.Start(); err != nil {
		log.Close()
		return Running{}, i18n.Error("launch.process_start_failed", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- errors.Join(cmd.Wait(), log.Close())
		close(done)
	}()
	return Running{PID: cmd.Process.Pid, Log: log.Name(), Done: done}, nil
}

type gameIdentity struct {
	name, uuid, token, clientID, xuid, kind string
}

func launchIdentity(offline string, session *MinecraftSession) (gameIdentity, error) {
	if session != nil {
		if !session.ValidFor(session.Profile.ID) || !profileID.MatchString(session.Profile.ID) || !instance.ValidPlayerName(session.Profile.Name) {
			return gameIdentity{}, ErrMicrosoftRequired
		}
		return gameIdentity{session.Profile.Name, session.Profile.ID, session.accessToken, microsoftClientID, session.xuid, "msa"}, nil
	}
	if !instance.ValidPlayerName(offline) {
		return gameIdentity{}, i18n.Error("account.offline_name_invalid")
	}
	uuid := md5.Sum([]byte("OfflinePlayer:" + offline))
	uuid[6] = uuid[6]&0x0f | 0x30
	uuid[8] = uuid[8]&0x3f | 0x80
	return gameIdentity{name: offline, uuid: hex.EncodeToString(uuid[:]), token: "0", kind: "legacy"}, nil
}

var (
	// ErrRepairRequired indicates that cached files cannot be used to start the client.
	ErrRepairRequired = i18n.Error("launch.repair_required")
	// ErrJavaRequired indicates that the selected game needs a usable Java runtime.
	ErrJavaRequired = i18n.Error("launch.java_required")
)

package minecraft

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

func (g Runtime) launchArguments(ctx context.Context, i instance.Instance, options LaunchOptions, profile launchProfile, memory int) ([]string, error) {
	m, cp, libraryDir, launchVersion := profile.metadata, profile.classpath, profile.libraryDir, profile.version
	identity, server := options.Identity, options.Server
	natives := filepath.Join(i.GameDir, ".lxmcl-natives")
	if err := os.MkdirAll(natives, 0755); err != nil {
		return nil, i18n.Error("launch.runtime_dir_create_failed", err)
	}
	values := map[string]string{
		"natives_directory": natives, "launcher_name": "LXMCL", "launcher_version": "0.2",
		"classpath": strings.Join(cp, string(os.PathListSeparator)), "classpath_separator": string(os.PathListSeparator),
		"library_directory": libraryDir,
		"version_name":      launchVersion, "game_directory": i.GameDir, "assets_root": filepath.Join(g.Cache, "assets"),
		"assets_index_name": m.AssetIndex.ID,
		"version_type":      m.Type,
		"user_properties":   "{}",
	}
	for key, value := range identity.Arguments() {
		values[key] = value
	}
	jvm, err := gameArgs(m.Arguments.JVM, values, nil)
	if err != nil {
		return nil, i18n.Error("launch.jvm_args_failed", err)
	}
	if i.UsesInstaller() {
		ignoreCachedVanillaJar(jvm, launchVersion)
	}
	game, err := clientArgs(m, values, i.Launch, server)
	if err != nil {
		return nil, i18n.Error("launch.client_args_failed", err)
	}
	args := append([]string{"-Xmx" + strconv.Itoa(memory) + "M"}, jvm...)
	if options.PrepareJVM != nil {
		extra, err := options.PrepareJVM(ctx)
		if err != nil {
			return nil, err
		}
		args = append(args, extra...)
	}
	if l := m.Logging.Client; l.Argument != "" {
		args = append(args, strings.ReplaceAll(l.Argument, "${path}", filepath.Join(g.Cache, "assets", "log_configs", l.File.ID)))
	}
	args = append(append(args, m.MainClass), game...)
	return args, nil
}

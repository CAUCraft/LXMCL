package launcher

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	shimLibrary = "test/runtime/patched/1/patched-1-client.jar"
	shimAux     = "test/runtime/extra/1/extra-1-client.jar"
	shimOutput  = "official installer output"
	shimMain    = "test.loader.Client"
)

// Reuse the test executable as a Java process to exercise the actual command,
// working directory, verified cache, modpack publication, and launch path on CI.
func TestMain(m *testing.M) {
	if os.Getenv("LXMCL_TEST_FORGE_PROCESS") == "1" && len(os.Args) > 1 {
		switch os.Args[1] {
		case "-XshowSettings:properties":
			if os.Getenv(launcherCurseForgeKey) != "" || os.Getenv(standardCurseForgeKey) != "" {
				os.Exit(2)
			}
			fmt.Println("    java.specification.version = 21")
			os.Exit(0)
		case "-Djava.awt.headless=true":
			if err := shimForgeInstall(os.Args[1:]); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			os.Exit(0)
		case "-Xmx2048M":
			args := os.Args[2:]
			main := slices.Index(args, shimMain)
			cp := slices.Index(args, "-cp")
			mods, _ := os.ReadFile("config/probe.txt")
			if cp < 0 || cp+1 >= len(args) || !strings.Contains(args[cp+1], "patched-1-client.jar") ||
				main < 0 || !slices.Contains(args[main+1:], "shimClient") || string(mods) != "isolated" ||
				!slices.ContainsFunc(args[:max(main, 0)], func(arg string) bool {
					return strings.HasPrefix(arg, "-DignoreList=") && strings.HasSuffix(arg, ".jar,client.jar")
				}) || os.Getenv("CURSEFORGE_API_KEY") != "" {
				os.Exit(2)
			}
			fmt.Println("shim loader and inherited Minecraft started")
			os.Exit(0)
		}
	}
	os.Exit(m.Run())
}

func shimForgeInstall(args []string) error {
	if len(args) != 6 || args[2] != "-jar" || args[4] != "--installClient" ||
		args[1] != "-Duser.home="+args[5] || os.Getenv("CURSEFORGE_API_KEY") != "" {
		return errors.New("installer was not run with the isolated client arguments")
	}
	installer, p, err := forgeMetadata(args[3])
	if err != nil {
		return err
	}
	dir := args[5]
	profile, err := os.ReadFile(filepath.Join(dir, "launcher_profiles.json"))
	if err != nil || string(profile) != `{"profiles":{}}` {
		return errors.New("installer did not get an isolated launcher profile")
	}
	jar, err := os.ReadFile(filepath.Join(dir, "versions", installer.Minecraft, installer.Minecraft+".jar"))
	if err != nil || string(jar) != "client" {
		return errors.New("installer did not get the verified vanilla jar")
	}
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if err := writeFile(filepath.Join(dir, "versions", p.ID, p.ID+".json"), b); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(dir, "libraries", filepath.FromSlash(shimLibrary)), []byte(shimOutput)); err != nil {
		return err
	}
	return writeFile(filepath.Join(dir, "libraries", filepath.FromSlash(shimAux)), []byte(shimOutput))
}

func TestPatchedModpackInstallAndLaunch(t *testing.T) {
	t.Setenv("LXMCL_TEST_FORGE_PROCESS", "1")
	t.Setenv(standardCurseForgeKey, "must-not-reach-java")
	t.Setenv(launcherCurseForgeKey, "must-not-reach-java")
	java, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		kind, version, entry, body, installerPath string
	}{
		{LoaderForge, "52.1.14", "modrinth.index.json",
			`{"formatVersion":1,"game":"minecraft","name":"Forge Test","dependencies":{"minecraft":"1.21.1","forge":"1.21.1-52.1.14"}}`,
			"net/minecraftforge/forge/1.21.1-52.1.14/forge-1.21.1-52.1.14-installer.jar"},
		{LoaderNeoForge, "21.1.200", "manifest.json",
			`{"manifestType":"minecraftModpack","manifestVersion":1,"name":"NeoForge Test",
			  "minecraft":{"version":"1.21.1","modLoaders":[{"id":"neoforge-21.1.200","primary":true}]}}`,
			"releases/net/neoforged/neoforge/21.1.200/neoforge-21.1.200-installer.jar"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			ctx := t.Context()
			file := "import.mrpack"
			if tc.kind == LoaderNeoForge {
				file = "import.zip"
			}
			pack, err := PrepareLocalModpack(ctx, packFile(t, file, map[string]string{
				tc.entry: tc.body, "overrides/config/probe.txt": "isolated",
			}))
			if err != nil {
				t.Fatal(err)
			}
			store, old, next, i := packState(t, pack.Info())
			next.Settings.JavaPath = java
			if i.Loader != tc.kind || i.LoaderVersion != tc.version || i.GameDir == old.Instances[0].GameDir {
				t.Fatalf("unexpected staged instance: %+v", i)
			}
			src, err := forgeInstallerURL(i)
			if err != nil || !strings.HasSuffix(src, "/"+tc.installerPath) {
				t.Fatalf("installer source: %q, %v", src, err)
			}
			profileID := i.Version + "-" + strings.ToLower(tc.kind) + "-" + tc.version
			installer := packArchive(t, map[string]string{
				"install_profile.json": fmt.Sprintf(`{"minecraft":"1.21.1","version":%q,"json":"/version.json",
					"data":{"MC_EXTRA":{"client":"[test.runtime:extra:1:client]"},"EXTRA_SHA":{"client":"'%x'"}},
					"processors":[{"sides":["client"],"outputs":{"{MC_EXTRA}":"{EXTRA_SHA}"}},
						{"sides":["server"],"outputs":{"[test.runtime:extra:1:server]":"'%x'"}}]}`,
					profileID, sha1.Sum([]byte(shimOutput)), sha1.Sum([]byte("server only"))),
				"version.json": fmt.Sprintf(`{"id":%q,"inheritsFrom":"1.21.1","mainClass":%q,
					"arguments":{"game":["--launchTarget","shimClient"],
						"jvm":["-Dloader.path=${library_directory}","-DignoreList=asm,${version_name}.jar"]},
					"libraries":[{"name":"test.runtime:patched:1:client","downloads":{"artifact":{"path":%q}}}]}`, profileID, shimMain, shimLibrary),
			})
			fixture := newInstallFixture(t)
			fixture.files[src] = installer
			fixture.files[src+".sha1"] = []byte(fmt.Sprintf("%x", sha1.Sum(installer)))
			client := &http.Client{Transport: transportFunc(fixture.request)}
			game := Game{Cache: t.TempDir()}
			install := func(ctx context.Context, work Instance, report func(Progress)) error {
				return game.installWithJava(ctx, work, next.Settings.JavaPath, report, &downloads{http: client})
			}
			if err := game.installModpack(ctx, store, next, i, pack, curseForge{}, client, install, nil); err != nil {
				t.Fatal(err)
			}
			state, err := store.Load()
			if err != nil || state.Selected != old.Selected || !game.Installed(i) {
				t.Fatalf("valid modded installation missing: %+v, %v", state, err)
			}
			run, err := game.Start(ctx, i, Settings{JavaPath: java, Player: "Player", Memory: 2048})
			if err != nil {
				t.Fatal(err)
			}
			if err := <-run.Done; err != nil {
				t.Fatal(err)
			}
			log, err := os.ReadFile(run.Log)
			if err != nil || !strings.Contains(string(log), "shim loader and inherited Minecraft started") {
				t.Fatalf("loader was not started: %s, %v", log, err)
			}
			// The processor output is discovered through library_directory,
			// even though it was never on the Java classpath.
			path := filepath.Join(game.forgeDir(i), "libraries", filepath.FromSlash(shimAux))
			if err := os.WriteFile(path, []byte(strings.Repeat("x", len(shimOutput))), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := game.Start(ctx, i, Settings{JavaPath: java, Player: "Player", Memory: 2048}); !errors.Is(err, ErrRepairRequired) {
				t.Fatalf("damaged loader library was not detected: %v", err)
			}
			if err := install(ctx, i, nil); err != nil {
				t.Fatalf("unable to repair loader: %v", err)
			}
			if b, err := os.ReadFile(path); err != nil || string(b) != shimOutput {
				t.Fatalf("repair did not replace corrupt library: %q, %v", b, err)
			}
		})
	}
}

func TestForgeClassPathRetainsNativeClassifier(t *testing.T) {
	var m metadata
	var p forgeProfile
	if err := json.Unmarshal([]byte(`{"libraries":[
		{"name":"org.example:library:1","downloads":{"artifact":{"path":"old.jar"}}},
		{"name":"org.example:library:1:natives-windows","downloads":{"artifact":{"path":"native.jar"}}}
	]}`), &m); err != nil {
		t.Fatal(err)
	}
	p.Libraries = []forgeLibrary{{Name: "org.example:library:2"}}
	g := Game{Cache: t.TempDir()}
	libraries := filepath.Join(g.Cache, "libraries")
	old, native := filepath.Join(libraries, "old.jar"), filepath.Join(libraries, "native.jar")
	got := g.mergeForgeClasspath(m, p, []string{old, native, "client.jar"}, []string{"new.jar"})
	if !slices.Equal(got, []string{"new.jar", native, "client.jar"}) {
		t.Fatalf("wrong merged classpath: %v", got)
	}
}

func TestLegacyForgePackLoaderCoordinates(t *testing.T) {
	for _, tc := range []struct {
		id, kind, version, url string
	}{
		{"forge-1.20.1-47.4.10", LoaderForge, "47.4.10",
			forgeMaven + "net/minecraftforge/forge/1.20.1-47.4.10/forge-1.20.1-47.4.10-installer.jar"},
		{"neoforge-47.1.106", LoaderNeoForge, "47.1.106",
			neoMaven + "net/neoforged/forge/1.20.1-47.1.106/forge-1.20.1-47.1.106-installer.jar"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			file := packFile(t, "legacy.zip", map[string]string{
				"manifest.json": fmt.Sprintf(`{"manifestType":"minecraftModpack","manifestVersion":1,
					"minecraft":{"version":"1.20.1","modLoaders":[{"id":%q,"primary":true}]}}`, tc.id),
			})
			pack, err := PrepareLocalModpack(t.Context(), file)
			if err != nil {
				t.Fatal(err)
			}
			info := pack.Info()
			src, err := forgeInstallerURL(Instance{Loader: info.Loader, LoaderVersion: info.LoaderVersion, Version: info.Minecraft})
			if err != nil || info.Loader != tc.kind || info.LoaderVersion != tc.version || src != tc.url {
				t.Fatalf("unexpected loader: %+v, %s, %v", info, src, err)
			}
		})
	}
}

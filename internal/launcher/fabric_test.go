package launcher

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestKnotInstallVerifiesLibrariesSeparatelyFromVanilla(t *testing.T) {
	for _, cfg := range []struct{ loader, version, meta, main, jar, coordinate, repo string }{
		{LoaderFabric, "0.16.14", fabricMeta, "net.fabricmc.loader.impl.launch.knot.KnotClient", "fabric-loader", "net.fabricmc", fabricMaven},
		{LoaderQuilt, "0.30.2", quiltMeta, "org.quiltmc.loader.impl.launch.knot.KnotClient", "quilt-loader", "org.quiltmc", "https://maven.quiltmc.org/repository/release/"},
	} {
		t.Run(cfg.loader, func(t *testing.T) {
			testKnotInstall(t, cfg.loader, cfg.version, cfg.meta, cfg.main, cfg.jar, cfg.coordinate, cfg.repo)
		})
	}
}

func testKnotInstall(t *testing.T, loader, version, meta, main, jar, group, repo string) {
	t.Helper()
	f := newInstallFixture(t)
	d := &downloads{http: &http.Client{Transport: transportFunc(f.request)}}
	g := Game{Cache: t.TempDir()}
	vanilla := Instance{Version: "1.21.1", GameDir: t.TempDir()}
	modified := vanilla
	modified.Loader, modified.LoaderVersion = loader, version
	fileURL := repo + strings.ReplaceAll(group, ".", "/") + "/" + jar + "/" + version + "/" + jar + "-" + version + ".jar"
	f.files[fileURL] = []byte(jar)
	f.files[fileURL+".sha1"] = []byte(fmt.Sprintf("%x", sha1.Sum(f.files[fileURL])))
	profile, err := json.Marshal(map[string]any{
		"id": jar + "-" + version + "-1.21.1", "inheritsFrom": "1.21.1",
		"mainClass": main,
		"libraries": []map[string]string{{"name": group + ":" + jar + ":" + version, "url": repo}},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.files[meta+"1.21.1/"+version+"/profile/json"] = profile
	if err := g.install(t.Context(), vanilla, nil, d); err != nil {
		t.Fatal(err)
	}
	if g.Installed(modified) {
		t.Fatal("vanilla receipt marked the loader as installed")
	}
	if err := g.install(t.Context(), modified, nil, d); err != nil {
		t.Fatal(err)
	}
	_, files, err := g.verifyKnot(t.Context(), modified)
	if err != nil || !g.Installed(modified) || len(files) != 1 {
		t.Fatalf("loader install was not launchable: %v, %v", files, err)
	}
	if err := os.WriteFile(files[0], []byte(strings.Repeat("x", len(jar))), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := g.verifyKnot(t.Context(), modified); err == nil {
		t.Fatal("accepted a corrupted loader library")
	}
	if _, _, err := g.verifyInstalled(t.Context(), vanilla); err != nil {
		t.Fatalf("loader damage affected vanilla: %v", err)
	}
	if err := g.install(t.Context(), modified, nil, d); err != nil {
		t.Fatal(err)
	}
	if _, _, err := g.verifyKnot(t.Context(), modified); err != nil {
		t.Fatalf("failed to repair the loader library: %v", err)
	}
}

package launcher

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
)

func TestFabricInstallVerifiesLibrariesSeparatelyFromVanilla(t *testing.T) {
	f := newInstallFixture(t)
	d := &downloads{http: &http.Client{Transport: transportFunc(f.request)}}
	g := Game{Cache: t.TempDir()}
	vanilla := Instance{Version: "1.21.1", GameDir: t.TempDir()}
	modified := vanilla
	modified.Loader, modified.LoaderVersion = "Fabric", "0.16.14"
	fileURL := "https://maven.fabricmc.net/net/fabricmc/fabric-loader/0.16.14/fabric-loader-0.16.14.jar"
	f.files[fileURL] = []byte("fabric-loader")
	f.files[fileURL+".sha1"] = []byte(fmt.Sprintf("%x", sha1.Sum(f.files[fileURL])))
	profile, err := json.Marshal(map[string]any{
		"id": "fabric-loader-0.16.14-1.21.1", "inheritsFrom": "1.21.1",
		"mainClass": "net.fabricmc.loader.impl.launch.knot.KnotClient",
		"libraries": []map[string]string{{"name": "net.fabricmc:fabric-loader:0.16.14", "url": "https://maven.fabricmc.net/"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.files[fabricMeta+"1.21.1/0.16.14/profile/json"] = profile
	if err := g.install(t.Context(), vanilla, nil, d); err != nil {
		t.Fatal(err)
	}
	if g.Installed(modified) {
		t.Fatal("vanilla receipt marked Fabric as installed")
	}
	if err := g.install(t.Context(), modified, nil, d); err != nil {
		t.Fatal(err)
	}
	_, files, err := g.verifyFabric(t.Context(), modified)
	if err != nil || !g.Installed(modified) || len(files) != 1 {
		t.Fatalf("Fabric install was not launchable: %v, %v", files, err)
	}
	if err := os.WriteFile(files[0], []byte("corrupt-data!"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := g.verifyFabric(t.Context(), modified); err == nil {
		t.Fatal("accepted a corrupted Fabric library")
	}
	if _, _, err := g.verifyInstalled(t.Context(), vanilla); err != nil {
		t.Fatalf("Fabric damage affected vanilla: %v", err)
	}
	if err := g.install(t.Context(), modified, nil, d); err != nil {
		t.Fatal(err)
	}
	if _, _, err := g.verifyFabric(t.Context(), modified); err != nil {
		t.Fatalf("failed to repair the Fabric library: %v", err)
	}
}

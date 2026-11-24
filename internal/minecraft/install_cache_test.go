package minecraft

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

func TestInstallCacheStatusSeparatesGameAndLoader(t *testing.T) {
	g := Runtime{Cache: t.TempDir(), Client: Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("cache check made a network request")
		return nil, nil
	})}}
	i := instance.Instance{Version: "1.21.1", GameDir: filepath.Join(t.TempDir(), "future-instance"), Loader: instance.LoaderFabric, LoaderVersion: "0.16.14"}
	check := func(target instance.Instance, want CacheStatus) {
		t.Helper()
		got, err := g.CacheStatus(target.Version, target.Loader, target.LoaderVersion)
		if err != nil || got != want || g.Installed(target) != (want.Game && want.Loader) {
			t.Fatalf("cache status = %+v, %v; installed = %v; want %+v", got, err, g.Installed(target), want)
		}
	}

	check(i, CacheStatus{})
	vanilla := i
	vanilla.Loader, vanilla.LoaderVersion = instance.LoaderVanilla, ""
	check(vanilla, CacheStatus{Loader: true})
	game := g.versionDir(i.Version)
	for _, name := range []string{"verified", "version.json", "client.jar"} {
		if err := storage.WriteFile(filepath.Join(game, name), []byte("1")); err != nil {
			t.Fatal(err)
		}
	}
	check(vanilla, CacheStatus{Game: true, Loader: true})
	check(i, CacheStatus{Game: true})

	_, _, main := knotSource(i)
	profile := []byte(`{"GameVersion":"1.21.1","LoaderVersion":"0.16.14","MainClass":"` + main + `","Libraries":[{"path":"loader.jar"}]}`)
	if err := storage.WriteFile(filepath.Join(g.knotDir(i), "verified.json"), profile); err != nil {
		t.Fatal(err)
	}
	check(i, CacheStatus{Game: true, Loader: true})
	if err := os.Remove(filepath.Join(game, "client.jar")); err != nil {
		t.Fatal(err)
	}
	check(i, CacheStatus{Loader: true})
	other := i
	other.LoaderVersion = "0.16.15"
	check(other, CacheStatus{})
	if _, err := g.CacheStatus(i.Version, i.Loader, ""); err == nil {
		t.Fatal("cache check accepted an incomplete loader choice")
	}
	if _, err := os.Stat(i.GameDir); !os.IsNotExist(err) {
		t.Fatalf("cache check touched the instance directory: %v", err)
	}
}

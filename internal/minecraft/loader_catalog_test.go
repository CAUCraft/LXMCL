package minecraft

import (
	"net/http"
	"slices"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/instance"
)

func TestLoaderCatalogFiltersMinecraftAndKeepsStableFirst(t *testing.T) {
	const neo = `<metadata><versioning><versions>
		<version>21.1.12-beta</version><version>21.1.9</version><version>21.10.20</version>
		<version>21.1.10</version><version>26.1.0.5-beta</version><version>26.1.0.3</version>
		<version>26.1.1.30</version><version>21.1.10</version>
	</versions></versioning></metadata>`
	const forge = `<metadata><versioning><versions>
		<version>1.21.1-52.0.9</version><version>1.21.1-52.0.10</version><version>1.21-51.0.8</version>
	</versions></versioning></metadata>`
	const quilt = `[{"loader":{"version":"0.28.0"}},{"loader":{"version":"0.29.2-beta.10"}},
		{"loader":{"version":"0.29.2-beta.9"}},{"loader":{"version":"0.30.2"}}]`
	entries := map[string]string{
		neoMaven + "net/neoforged/neoforge/maven-metadata.xml":     neo,
		forgeMaven + "net/minecraftforge/forge/maven-metadata.xml": forge,
		quiltMeta + "1.21.1": quilt,
	}
	d := &downloads{http: &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		value, ok := entries[req.URL.String()]
		if !ok {
			t.Fatalf("unexpected source: %s", req.URL)
		}
		return fixtureResponse(http.StatusOK, []byte(value)), nil
	})}}
	for _, tc := range []struct {
		kind, mc string
		want     []LoaderRelease
	}{
		{instance.LoaderForge, "1.21.1", []LoaderRelease{{"52.0.10", false}, {"52.0.9", false}}},
		{instance.LoaderNeoForge, "1.21.1", []LoaderRelease{{"21.1.10", false}, {"21.1.9", false}, {"21.1.12-beta", true}}},
		{instance.LoaderNeoForge, "26.1", []LoaderRelease{{"26.1.0.3", false}, {"26.1.0.5-beta", true}}},
		{instance.LoaderQuilt, "1.21.1", []LoaderRelease{{"0.30.2", false}, {"0.28.0", false}, {"0.29.2-beta.10", true}, {"0.29.2-beta.9", true}}},
	} {
		got, err := loaderVersions(t.Context(), tc.mc, tc.kind, d)
		if err != nil || !slices.Equal(got, tc.want) {
			t.Fatalf("%s %s: %+v, %v", tc.kind, tc.mc, got, err)
		}
	}
}

package minecraft

import (
	"testing"

	"github.com/CAUCraft/LXMCL/internal/instance"
)

func TestLegacyForgeInstallerCoordinates(t *testing.T) {
	for _, tc := range []struct{ kind, version, url string }{
		{instance.LoaderForge, "1.20.1-47.4.10", forgeMaven + "net/minecraftforge/forge/1.20.1-47.4.10/forge-1.20.1-47.4.10-installer.jar"},
		{instance.LoaderNeoForge, "47.1.106", neoMaven + "net/neoforged/forge/1.20.1-47.1.106/forge-1.20.1-47.1.106-installer.jar"},
	} {
		got, err := forgeInstallerURL(instance.Instance{Version: "1.20.1", Loader: tc.kind, LoaderVersion: tc.version})
		if err != nil || got != tc.url {
			t.Fatalf("%s source: %q %v", tc.kind, got, err)
		}
	}
}

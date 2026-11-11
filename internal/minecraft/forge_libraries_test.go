package minecraft

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"
)

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
	g := Runtime{Cache: t.TempDir()}
	libraries := filepath.Join(g.Cache, "libraries")
	old, native := filepath.Join(libraries, "old.jar"), filepath.Join(libraries, "native.jar")
	got := g.mergeForgeClasspath(m, p, []string{old, native, "client.jar"}, []string{"new.jar"})
	if !slices.Equal(got, []string{"new.jar", native, "client.jar"}) {
		t.Fatalf("wrong merged classpath: %v", got)
	}
}

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCurseForgeEnvKeepsKeyLiteralAndRespectsEnvironment(t *testing.T) {
	t.Setenv(launcherCurseForgeKey, "")
	t.Setenv(standardCurseForgeKey, "")
	dir := t.TempDir()
	const want = "$test$example/token"
	data := "\ufeff# Local configuration\r\nCURSEFORGE_API_KEY=alternate\r\nLXMCL_CURSEFORGE_API_KEY='" + want + "'\r\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if LoadCurseForgeAPIKey(dir) != want || os.Getenv(launcherCurseForgeKey) != "" || os.Getenv(standardCurseForgeKey) != "" {
		t.Fatal("the local key was expanded, skipped, or copied into the process environment")
	}
	t.Setenv(standardCurseForgeKey, "env-override")
	if LoadCurseForgeAPIKey(dir) != "env-override" {
		t.Fatal("an environment variable should override the local file")
	}
}

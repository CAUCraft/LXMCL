package log

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGameLogPreviewReadsBoundedTail(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "launcher-logs")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "game-test.log")
	prefix := "this line must not be shown"
	b := []byte(prefix + "\n" + strings.Repeat("一次游戏事件\n", tailLimit) + "客户端退出\n")
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	log, err := ReadGame(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(log, prefix) || !strings.HasSuffix(log, "客户端退出\n") || len(log) > tailLimit+128 {
		t.Fatalf("unexpected log preview of %d bytes", len(log))
	}
	if _, err := ReadGame(filepath.Join(dir, "other.exe")); err == nil {
		t.Fatal("accepted a non-log path")
	}
}

func TestInstallerLogPreservesLocationAndOutput(t *testing.T) {
	root := t.TempDir()
	f, err := CreateInstaller(root, "NeoForge")
	if err != nil {
		t.Fatal(err)
	}
	const output = "installer output\n"
	_, writeErr := f.WriteString(output)
	if err := f.Close(); err != nil || writeErr != nil {
		t.Fatalf("write installer log: %v, %v", writeErr, err)
	}
	dir, name := filepath.Split(f.Name())
	if filepath.Clean(dir) != filepath.Join(root, "installer-logs") ||
		!strings.HasPrefix(name, "neoforge-") || filepath.Ext(name) != ".log" {
		t.Fatalf("unexpected installer log path %q", f.Name())
	}
	if b, err := os.ReadFile(f.Name()); err != nil || string(b) != output {
		t.Fatalf("installer log contains %q, %v", b, err)
	}
}

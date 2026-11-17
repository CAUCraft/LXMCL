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

func TestLogFilterKeepsExceptionContext(t *testing.T) {
	const ordinary = "[12:00:00] [main/INFO] Starting game\n"
	const failure = "[12:00:01] [main/ERROR] Startup failed\njava.lang.OutOfMemoryError: heap space\n\tat demo.Client.start(Client.java:42)\nCaused by: java.lang.RuntimeException\n"
	const warning = "[12:00:02] [main/WARN] Optional pack missing\n"
	text := ordinary + failure + warning
	if got := Filter(text, "", true); got != failure+warning {
		t.Fatalf("severity filter lost context: %q", got)
	}
	if got := Filter(text, "client.java", true); got != failure {
		t.Fatalf("stack search lost its event: %q", got)
	}
	if got := Filter(text, "no match", false); got != "" {
		t.Fatalf("unmatched query returned %q", got)
	}
	if got := Filter(text, "", false); got != text {
		t.Fatal("unfiltered preview changed")
	}
}

func TestExportPreviewPreservesSourceAndWritesOnlyCapturedText(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "game.log")
	if err := os.WriteFile(source, []byte("original game log"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ExportPreview(source, source, "filtered"); err == nil {
		t.Fatal("export replaced its source")
	}
	alias := filepath.Join(dir, "alias.txt")
	if err := os.Link(source, alias); err != nil {
		t.Fatal(err)
	}
	if err := ExportPreview(source, alias, "filtered"); err == nil {
		t.Fatal("export replaced a source alias")
	}
	destination := filepath.Join(dir, "preview.txt")
	const preview = "[main/ERROR] 测试失败\n\tat demo.Client.start(Client.java:42)\n"
	if err := ExportPreview(source, destination, preview); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(destination); err != nil || string(b) != preview {
		t.Fatalf("export changed the captured preview: %q, %v", b, err)
	}
	if b, err := os.ReadFile(source); err != nil || string(b) != "original game log" {
		t.Fatalf("export changed the source: %q, %v", b, err)
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

package log

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRecentDiagnosticsAreBoundedAndKeepInstanceFiles(t *testing.T) {
	dir := t.TempDir()
	write := func(name, text string, modified time.Time) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatal(err)
		}
		return path
	}
	now := time.Now()
	for n := range RecentLimit + 5 {
		write(fmt.Sprintf("launcher-logs/game-%02d.log", n), "launch", now.Add(-time.Duration(n+1)*time.Hour))
	}
	latest := write("logs/latest.log", "latest game output", now)
	crash := write("crash-reports/crash-demo-client.txt", "java.lang.RuntimeException: crash", now.Add(time.Second))
	write("logs/credentials.txt", "must not list", now.Add(time.Minute))
	var compressed bytes.Buffer
	z := gzip.NewWriter(&compressed)
	end := "[main/ERROR] archived failure\n"
	if _, err := z.Write([]byte(strings.Repeat("prior entry\n", tailLimit) + end)); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	archive := write("logs/2026-11-18-1.log.gz", compressed.String(), now.Add(time.Minute))
	write("logs/unrelated.log.gz", "unrelated", now.Add(time.Hour))
	debug := write("logs/debug.log", "", now.Add(-time.Second))
	if got, err := ReadPreview(t.Context(), debug); err != nil || got != "" {
		t.Fatalf("empty log preview: %q, %v", got, err)
	}
	if got, err := ReadPreview(t.Context(), archive); err != nil || !strings.HasSuffix(got, end) || len(got) > tailLimit+128 {
		t.Fatalf("archive tail: %d bytes, %v", len(got), err)
	}
	// A bad gzip footer must not present partial text as a successful preview.
	broken := write("logs/debug-1.log.gz", compressed.String()[:compressed.Len()-1], now.Add(-time.Hour))
	if got, err := ReadPreview(t.Context(), broken); err == nil || got != "" {
		t.Fatal("incomplete archive returned a preview")
	}
	entries, err := Recent(t.Context(), dir)
	if err != nil || len(entries) != RecentLimit || entries[0].Path != archive || entries[1].Path != crash || entries[2].Path != latest || entries[3].Path != debug {
		t.Fatalf("unexpected recent logs: %v, %v", entries, err)
	}
	if got, err := ReadPreview(t.Context(), crash); err != nil || got != "java.lang.RuntimeException: crash" {
		t.Fatalf("crash preview: %q, %v", got, err)
	}
	large := "java.lang.RuntimeException: crash\n" + strings.Repeat("environment details\n", tailLimit) + "omitted tail"
	if err := os.WriteFile(crash, []byte(large), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadPreview(t.Context(), crash); err != nil || !strings.HasPrefix(got, "java.lang.RuntimeException: crash") || strings.Contains(got, "omitted tail") || len(got) > tailLimit+128 {
		t.Fatalf("large crash report lost its cause or exceeded the bound: %d bytes, %v", len(got), err)
	}
	if _, err := ReadGame(crash); err == nil {
		t.Fatal("legacy game-log validation accepted a crash report")
	}
	if _, err := ReadPreview(t.Context(), filepath.Join(dir, "logs", "credentials.txt")); err == nil {
		t.Fatal("preview accepted a non-diagnostic file")
	}
	if err := ExportPreview(latest, crash, "replace"); err == nil {
		t.Fatal("export overwrote another diagnostic file")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Recent(ctx, dir); !errors.Is(err, context.Canceled) {
		t.Fatalf("scan ignored cancellation: %v", err)
	}
	if entries, err := Recent(t.Context(), filepath.Join(dir, "new-instance")); err != nil || len(entries) != 0 {
		t.Fatalf("new instance failed: %v, %v", entries, err)
	}
}

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

func TestFilterCrashReportSections(t *testing.T) {
	head := "---- Minecraft Crash Report ----\r\njava.lang.IllegalStateException: startup\r\n"
	level := "-- Affected level --\r\n[World information]\r\nLevel name: Workshop\r\n"
	system := "-- System Details --\r\nJava Version: 21\r\n"
	report := head + level + system
	if got := Filter(report, "workshop", false); got != level {
		t.Fatalf("level section: %q", got)
	}
	if got := Filter(report, "", true); got != head {
		t.Fatalf("exception section: %q", got)
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

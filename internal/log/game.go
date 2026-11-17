package log

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

const (
	GameDirectory = "launcher-logs"
	gamePrefix    = "game-"
	tailLimit     = 64 << 10
	archiveLimit  = 32 << 20
)

// GameHeader records the runtime without logging launch arguments or tokens.
type GameHeader struct {
	Version, JavaPath    string
	JavaMajor, MemoryMiB int
}

func CreateGame(gameDir string, h GameHeader) (*os.File, error) {
	dir := filepath.Join(gameDir, GameDirectory)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, i18n.Error("log.game_directory_create_failed", err)
	}
	f, err := os.CreateTemp(dir, gamePrefix+"*.log")
	if err != nil {
		return nil, i18n.Error("log.game_create_failed", err)
	}
	if _, err := fmt.Fprintf(f, "[LXMCL] Minecraft %s · Java %d (%s) · -Xmx%dM\n", h.Version, h.JavaMajor, h.JavaPath, h.MemoryMiB); err != nil {
		f.Close()
		return nil, i18n.Error("log.game_write_failed", err)
	}
	return f, nil
}

func IsGame(path string) bool {
	base := filepath.Base(path)
	return filepath.IsAbs(path) && filepath.Base(filepath.Dir(path)) == GameDirectory &&
		strings.HasPrefix(base, gamePrefix) && filepath.Ext(base) == ".log"
}

func ReadGame(path string) (string, error) {
	if !IsGame(path) {
		return "", i18n.Error("log.game_path_invalid")
	}
	return ReadPreview(context.Background(), path)
}

func ReadPreview(ctx context.Context, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !IsPreview(path) {
		return "", i18n.Error("log.game_path_invalid")
	}
	root, err := os.OpenRoot(filepath.Dir(filepath.Dir(path)))
	if err != nil {
		return "", err
	}
	defer root.Close()
	name := filepath.Join(filepath.Base(filepath.Dir(path)), filepath.Base(path))
	info, err := root.Lstat(name)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", i18n.Error("log.game_not_regular")
	}
	f, err := root.Open(name)
	if err != nil {
		return "", err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !st.Mode().IsRegular() {
		return "", i18n.Error("log.game_not_regular")
	}
	crash := filepath.Base(filepath.Dir(path)) == "crash-reports"
	var b []byte
	switch {
	case strings.HasSuffix(name, ".log.gz"):
		if st.Size() > archiveLimit {
			return "", i18n.Error("log.archive_limit")
		}
		b, err = archivedTail(ctx, io.NewSectionReader(f, 0, st.Size()))
	case crash:
		b, err = io.ReadAll(io.LimitReader(f, tailLimit))
	default:
		// Include the preceding byte so a complete first line is preserved.
		offset := max(st.Size()-(tailLimit+1), 0)
		b, err = io.ReadAll(io.NewSectionReader(f, offset, st.Size()-offset))
	}
	if err != nil {
		return "", err
	}
	truncated := len(b) > tailLimit
	if truncated {
		preceding := b[0]
		b = b[1:]
		if preceding != '\n' {
			if _, tail, ok := bytes.Cut(b, []byte{'\n'}); ok && len(tail) > 0 {
				b = tail
			}
		}
	}
	log := strings.ToValidUTF8(string(b), "\uFFFD")
	if log == "" {
		return i18n.Text("log.game_empty"), nil
	}
	if truncated {
		return i18n.Text("log.game_tail") + log, nil
	}
	if crash && st.Size() > tailLimit {
		return log + i18n.Text("log.crash_head"), nil
	}
	return log, nil
}

// Keep the same tail and preceding byte as a plain log, regardless of archive size.
func archivedTail(ctx context.Context, source io.Reader) ([]byte, error) {
	reader, err := gzip.NewReader(source)
	if err != nil {
		return nil, i18n.Error("log.archive_read_failed", err)
	}
	defer reader.Close()
	buf := make([]byte, tailLimit+1)
	total, pos := 0, 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, err := reader.Read(buf[pos:])
		total += n
		if total > archiveLimit {
			return nil, i18n.Error("log.archive_limit")
		}
		pos = (pos + n) % len(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, i18n.Error("log.archive_read_failed", err)
		}
	}
	if total < len(buf) {
		return buf[:total], nil
	}
	return append(buf[pos:], buf[:pos]...), nil
}

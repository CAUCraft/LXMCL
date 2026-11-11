package log

import (
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
	f, err := os.Open(path)
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
	offset := max(st.Size()-tailLimit, 0)
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return "", err
	}
	b, err := io.ReadAll(io.LimitReader(f, tailLimit))
	if err != nil {
		return "", err
	}
	if offset != 0 {
		if _, tail, ok := strings.Cut(string(b), "\n"); ok {
			b = []byte(tail)
		}
	}
	log := strings.ToValidUTF8(string(b), "\uFFFD")
	if log == "" {
		return i18n.Text("log.game_empty"), nil
	}
	if offset != 0 {
		return i18n.Text("log.game_tail") + log, nil
	}
	return log, nil
}

package launcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/modpack"
	"github.com/CAUCraft/LXMCL/internal/tasks"
)

func feedHash(address string) string {
	if address == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(address)))
	return hex.EncodeToString(sum[:])
}
func PackUpdateReference(i Instance, p PreparedModpack) tasks.PackUpdate {
	path, digest := p.ArchiveIdentity()
	feed, _ := p.Feed()
	return tasks.PackUpdate{Archive: path, SHA256: digest, Source: string(p.Info().Source), GameDir: i.GameDir, FeedHash: feedHash(feed)}
}
func PackRetryTarget(row tasks.Record, s State) (Instance, bool) {
	i, ok := s.Find(row.InstanceID)
	p := row.Pack
	return i, ok && (row.Status == tasks.Failed || row.Status == tasks.Canceled) && row.Revision == i.Revision && row.Version == i.Version && p.GameDir == i.GameDir && filepath.IsAbs(p.Archive) &&
		(p.Source == string(modpack.Modrinth) || p.Source == string(modpack.CurseForge)) && len(p.SHA256) == sha256.Size*2
}
func MatchesPackFeed(p tasks.PackUpdate, address string) bool { return feedHash(address) == p.FeedHash }
func ReopenPackUpdate(ctx context.Context, p tasks.PackUpdate, path, feed string) (PreparedModpack, error) {
	if !MatchesPackFeed(p, feed) {
		return PreparedModpack{}, i18n.Error("modpack.original_feed_required")
	}
	return modpack.Reopen(ctx, path, modpack.Source(p.Source), p.SHA256, strings.TrimSpace(feed))
}
func RollbackDirectory(err error) string {
	var e *modpack.RollbackError
	if errors.As(err, &e) {
		return e.Directory
	}
	return ""
}
func CanOpenPackBackup(p tasks.PackUpdate) bool {
	return filepath.IsAbs(p.GameDir) && filepath.Dir(p.BackupDir) == p.GameDir && strings.HasPrefix(filepath.Base(p.BackupDir), ".lxmcl-rollback-") && filepath.Clean(p.BackupDir) == p.BackupDir
}

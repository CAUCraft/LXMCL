package tasks

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

const (
	ModUpdateAction = "更新模组"
	ModUpdateNotRun = "未执行"

	// Leave room in the existing history limit for all 24 reports and other fields.
	maxModUpdateReportBytes = 48 << 10
	maxModUpdateReportItems = 160
)

type ModUpdateItem struct {
	File   string `json:"file,omitempty"`
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
	Status string `json:"status"`
}

type ModUpdateReport struct {
	Items   []ModUpdateItem `json:"items,omitempty"`
	Omitted int             `json:"omitted,omitempty"`
}

// SummarizeModUpdates keeps display metadata and outcomes. It omits
// oversized reports' tails without changing which files were attempted.
func SummarizeModUpdates(items []ModUpdateItem) ModUpdateReport {
	var report ModUpdateReport
	bytes := 0
	for _, item := range items {
		if !storage.ValidFilename(item.File) {
			item.File = ""
		}
		item.From, item.To = modVersionLabel(item.From), modVersionLabel(item.To)
		encoded, _ := json.Marshal(item) // Strings only.
		bytes += len(encoded) + 80       // Allow for indentation and separators in tasks.json.
		if len(report.Items) >= maxModUpdateReportItems || bytes > maxModUpdateReportBytes {
			break
		}
		report.Items = append(report.Items, item)
	}
	report.Omitted = len(items) - len(report.Items)
	return report
}

const (
	modUpdateBackupPrefix = "更新前备份 "
	modUpdateBackupSuffix = " 项模组"
)

// ModUpdateBackupAction describes the backup phase of a batch update.
func ModUpdateBackupAction(count int) string {
	return fmt.Sprintf("%s%d%s", modUpdateBackupPrefix, count, modUpdateBackupSuffix)
}

// ModUpdateBackupPath returns the recorded attachment without filesystem access.
func (r Record) ModUpdateBackupPath() string {
	if r.InstanceID <= 0 || r.ID == "" {
		return ""
	}
	batch := r.Action == ModUpdateAction && (r.Status == Completed || r.Status == Failed || r.Status == Canceled)
	backup := r.Status == Completed && strings.HasPrefix(r.Action, modUpdateBackupPrefix) && strings.HasSuffix(r.Action, modUpdateBackupSuffix)
	if !batch && !backup {
		return ""
	}
	return r.ModUpdateBackup.zipPath()
}

// LocateModUpdateBackup verifies the original ZIP at its recorded path.
func (r Record) LocateModUpdateBackup() (string, error) {
	filename := r.ModUpdateBackupPath()
	if filename == "" || storage.CheckFileStamp(filename, r.ModUpdateBackup.Stamp) != nil {
		return "", i18n.Error("resource.mod_update_backup_unavailable")
	}
	return filename, nil
}

func modVersionLabel(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 96 || strings.ContainsAny(value, "/\\:@") || strings.ContainsFunc(value, unicode.IsControl) {
		return ""
	}
	return value
}

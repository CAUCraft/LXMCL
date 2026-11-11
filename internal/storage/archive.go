package storage

import (
	"archive/zip"
	"encoding/json"
	"io"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

// ReadArchiveJSON reads one bounded, complete JSON document from a ZIP entry.
func ReadArchiveJSON(f *zip.File, limit int64, dst any) error {
	if f == nil {
		return i18n.Error("storage.archive_json_missing")
	}
	if limit <= 0 || f.UncompressedSize64 > uint64(limit) {
		return i18n.Error("storage.archive_json_too_large")
	}
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > limit {
		return i18n.Error("storage.archive_json_too_large")
	}
	return json.Unmarshal(data, dst)
}

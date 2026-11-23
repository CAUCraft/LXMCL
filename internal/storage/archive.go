package storage

import (
	"archive/zip"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

const (
	MaxPackSize    = 1 << 30
	MaxPackBytes   = 16 << 30
	MaxPackEntries = 20000
)

// OpenPackFile opens a regular archive within the compressed size limit.
func OpenPackFile(filename string) (*os.File, int64, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, 0, err
	}
	info, err := f.Stat()
	if err == nil && (!info.Mode().IsRegular() || info.Size() == 0 || info.Size() > MaxPackSize) {
		err = i18n.Error("modpack.archive_invalid")
	}
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	return f, info.Size(), nil
}

// IndexPackEntries validates portable entry names, types and declared sizes
// before any files are opened or extracted.
func IndexPackEntries(entries []*zip.File, maxFileSize uint64) (map[string]*zip.File, error) {
	if len(entries) > MaxPackEntries {
		return nil, i18n.Error("modpack.archive_too_many_files")
	}
	files := make(map[string]*zip.File, len(entries))
	seen := make(map[string]bool, len(entries))
	var bytes uint64
	for _, f := range entries {
		dir := f.Mode().Type() == fs.ModeDir && strings.HasSuffix(f.Name, "/")
		name := strings.TrimSuffix(f.Name, "/")
		key := strings.ToLower(name)
		if !ValidPortablePath(name) || seen[key] || (!dir && (!f.Mode().IsRegular() || f.Name != name)) {
			return nil, i18n.Error("modpack.archive_path_invalid", f.Name)
		}
		seen[key] = true
		if f.UncompressedSize64 > maxFileSize || f.UncompressedSize64 > MaxPackBytes-bytes {
			return nil, i18n.Error("modpack.archive_expanded_too_large")
		}
		bytes += f.UncompressedSize64
		if !dir {
			files[name] = f
		}
	}
	return files, nil
}

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

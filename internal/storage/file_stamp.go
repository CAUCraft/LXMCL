package storage

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

// FileStamp records the identity and metadata of a local regular file across restarts.
type FileStamp struct {
	System     string `json:"system"`
	Volume     uint64 `json:"volume"`
	File       uint64 `json:"file"`
	Size       int64  `json:"size"`
	ModifiedNS int64  `json:"modified_ns"`
	ChangedNS  int64  `json:"changed_ns"`
}

func (s FileStamp) Valid() bool {
	return (s.System == "linux" || s.System == "windows") && s.System == runtime.GOOS && s.File != 0 && s.Size > 0
}

// PublishNewStamped returns a stamp only if it can verify the original published file.
// A zero stamp with a nil error still means publication succeeded.
func PublishNewStamped(ctx context.Context, root *os.Root, name string, perm fs.FileMode, prepare func(*os.File) error) (FileStamp, error) {
	var pin *os.File
	var size int64
	err := publishNewObserved(ctx, root, name, perm, prepare, func(stage string, file *os.File) {
		expected, err := file.Stat()
		if err != nil {
			return
		}
		staged, err := root.Open(stage)
		if err != nil {
			return
		}
		info, err := staged.Stat()
		if err != nil || !info.Mode().IsRegular() || !os.SameFile(expected, info) {
			staged.Close()
			return
		}
		pin, size = staged, expected.Size()
	})
	if pin != nil {
		defer pin.Close()
	}
	if err != nil || pin == nil {
		return FileStamp{}, err
	}
	// The writer is closed and any staging link is gone. Read final timestamps
	// from the pinned object, never from a potentially replaced destination.
	stamp, info, err := stampFile(pin)
	if err != nil || stamp.Size != size || !sameStampedPath(root, name, info) {
		return FileStamp{}, nil
	}
	return stamp, nil
}

// CheckFileStamp validates the exact saved path without reading file contents.
func CheckFileStamp(filename string, expected FileStamp) error {
	if !expected.Valid() || !filepath.IsAbs(filename) || !ValidFilename(filepath.Base(filename)) {
		return fs.ErrInvalid
	}
	root, err := os.OpenRoot(filepath.Dir(filename))
	if err != nil {
		return err
	}
	defer root.Close()
	name := filepath.Base(filename)
	entry, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if !entry.Mode().IsRegular() {
		return fs.ErrInvalid
	}
	file, err := root.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	stamp, info, err := stampFile(file)
	if err != nil {
		return err
	}
	if stamp != expected || !os.SameFile(entry, info) || !sameStampedPath(root, name, info) {
		return fs.ErrInvalid
	}
	return nil
}

func stampFile(file *os.File) (FileStamp, fs.FileInfo, error) {
	info, err := file.Stat()
	if err != nil {
		return FileStamp{}, nil, err
	}
	if !info.Mode().IsRegular() {
		return FileStamp{}, nil, fs.ErrInvalid
	}
	volume, id, changed, err := fileStampIdentity(file, info)
	if err != nil {
		return FileStamp{}, nil, err
	}
	stamp := FileStamp{System: runtime.GOOS, Volume: volume, File: id, Size: info.Size(), ModifiedNS: info.ModTime().UnixNano(), ChangedNS: changed}
	if !stamp.Valid() {
		return FileStamp{}, nil, fs.ErrInvalid
	}
	return stamp, info, nil
}

func sameStampedPath(root *os.Root, name string, expected fs.FileInfo) bool {
	current, err := root.Lstat(name)
	return err == nil && current.Mode().IsRegular() && os.SameFile(expected, current) &&
		current.Size() == expected.Size() && current.ModTime().Equal(expected.ModTime())
}

package launcher

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCheckNewInstanceDirectoryIsReadOnly(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "missing")
	target := filepath.Join(parent, "instance-1")
	check := func(name string, wantOccupied, wantError bool) {
		t.Helper()
		occupied, err := CheckNewInstanceDirectory(name)
		if occupied != wantOccupied || (err != nil) != wantError {
			t.Fatalf("check %q = %v, %v", name, occupied, err)
		}
	}

	check("relative", false, true)
	check(target, false, false)
	if _, err := os.Lstat(parent); !os.IsNotExist(err) {
		t.Fatalf("checking created the missing parent: %v", err)
	}
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	check(target, true, false)
	file := filepath.Join(root, "existing")
	if err := os.WriteFile(file, []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	check(file, true, false)
	check(filepath.Join(file, "instance-1"), false, true)
	if data, err := os.ReadFile(file); err != nil || string(data) != "existing" {
		t.Fatalf("checking changed the existing file: %s, %v", data, err)
	}

	t.Run("dangling link", func(t *testing.T) {
		link := filepath.Join(root, "link")
		if err := os.Symlink(filepath.Join(root, "absent"), link); err != nil {
			t.Skipf("cannot create a link in this environment: %v", err)
		}
		occupied, err := CheckNewInstanceDirectory(link)
		if err != nil || !occupied {
			t.Fatalf("destination link = %t, %v", occupied, err)
		}
		if occupied, err := CheckNewInstanceDirectory(filepath.Join(link, "instance-1")); err == nil || occupied {
			t.Fatalf("candidate under dangling link = %t, %v", occupied, err)
		}
	})
}

func TestAvailableDiskSpaceIsReadOnly(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "尚未创建")
	target := filepath.Join(parent, "instance-1")
	check := func(t *testing.T, path string) {
		t.Helper()
		bytes, err := AvailableDiskSpace(path)
		if runtime.GOOS == "linux" || runtime.GOOS == "windows" {
			if err != nil {
				t.Fatalf("available bytes for %q = %d, %v", path, bytes, err)
			}
		} else if !errors.Is(err, errors.ErrUnsupported) {
			t.Fatalf("available bytes for %q = %d, %v, want unsupported", path, bytes, err)
		}
	}

	check(t, root)
	check(t, target)
	if got, err := nearestInstanceDirectory(target); got != root || err != nil {
		t.Fatalf("nearest directory for %q = %q, %v, want %q", target, got, err, root)
	}
	if _, err := os.Lstat(parent); !os.IsNotExist(err) {
		t.Fatalf("checking created the missing parent: %v", err)
	}
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"relative", file, filepath.Join(file, "instance-1")} {
		if bytes, err := AvailableDiskSpace(path); err == nil {
			t.Fatalf("available bytes for invalid path %q = %d, no error", path, bytes)
		}
	}

	t.Run("linked directory", func(t *testing.T) {
		dest := t.TempDir()
		link := filepath.Join(root, "linked")
		if err := os.Symlink(dest, link); err != nil {
			t.Skipf("cannot create a link in this environment: %v", err)
		}
		target := filepath.Join(link, "instance-1")
		check(t, target)
		if got, err := nearestInstanceDirectory(target); got != link || err != nil {
			t.Fatalf("nearest directory for %q = %q, %v, want %q", target, got, err, link)
		}
	})
}

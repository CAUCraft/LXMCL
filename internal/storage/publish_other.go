//go:build !windows

package storage

import (
	"context"
	"os"
)

func publishNew(ctx context.Context, root *os.Root, stage, name string, file *os.File) error {
	if err := file.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Link creates one directory entry atomically and fails if it already exists.
	// Unsupported filesystems fail without falling back to an overwriting rename.
	return root.Link(stage, name)
}

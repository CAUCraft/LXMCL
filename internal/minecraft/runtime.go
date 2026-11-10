// Package minecraft owns verified game installations and Java process startup.
// It does not select instances, authorize accounts, or persist launcher settings.
package minecraft

import (
	"regexp"

	"github.com/CAUCraft/LXMCL/internal/tasks"
)

var versionID = regexp.MustCompile(`^[0-9][0-9A-Za-z._-]{0,63}$`)

// Runtime uses Cache for shared game files. Callers serialize writes to the same
// cache and instance; all operations run synchronously in the caller's worker.
type Runtime struct {
	Cache  string
	Client Client
}
type Progress = tasks.Progress

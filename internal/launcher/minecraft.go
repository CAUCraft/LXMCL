package launcher

import (
	"regexp"

	"github.com/CAUCraft/LXMCL/internal/tasks"
)

var (
	versionID = regexp.MustCompile(`^[0-9][0-9A-Za-z._-]{0,63}$`)
)

type Game struct {
	Cache string
}

type Progress = tasks.Progress

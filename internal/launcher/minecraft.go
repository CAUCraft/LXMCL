package launcher

import (
	"regexp"
)

var (
	versionID = regexp.MustCompile(`^[0-9][0-9A-Za-z._-]{0,63}$`)
)

type Game struct {
	Cache string
}

type Progress struct {
	Stage       string
	Done, Total int
}

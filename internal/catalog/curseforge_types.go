package catalog

import (
	"time"
)

const (
	curseGameID        = 432
	curseModpacksClass = 4471
)

type curseProject struct {
	ID, GameID, ClassID  int
	Name, Summary, Slug  string
	IsAvailable          bool
	AllowModDistribution *bool
	Logo                 struct{ ThumbnailURL, URL string }
	Authors              []struct{ Name string }
}

type curseFile struct {
	ID, ModID, GameID, ReleaseType                  int
	DisplayName, FileName, DownloadURL              string
	FileLength                                      int64
	IsAvailable, IsServerPack, IsEarlyAccessContent bool
	FileDate                                        time.Time
	GameVersions                                    []string
	Hashes                                          []struct {
		Algo  int
		Value string
	}
}

type cursePage[T any] struct {
	Data       []T
	Pagination struct{ Index, ResultCount, TotalCount int }
}

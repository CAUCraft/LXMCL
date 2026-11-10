package launcher

import (
	"slices"

	"github.com/CAUCraft/LXMCL/internal/instance"
)

type Settings = instance.Settings
type Instance = instance.Instance

type State struct {
	MicrosoftProfile string           `json:"microsoft_profile,omitempty"`
	StationKey       string           `json:"skin_station_key,omitempty"`
	StationProfiles  []StationProfile `json:"skin_station_profiles,omitempty"`
	Servers          []Server         `json:"servers,omitempty"`
	Settings         Settings         `json:"settings"`
	Instances        []Instance       `json:"instances"`
	OfflinePlayers   []string         `json:"offline_players,omitempty"`
	Selected         int              `json:"selected"`
	NextID           int              `json:"next_id"`
}

func (s State) Clone() State {
	s.Instances = slices.Clone(s.Instances)
	s.OfflinePlayers = slices.Clone(s.OfflinePlayers)
	s.Servers = slices.Clone(s.Servers)
	s.StationProfiles = slices.Clone(s.StationProfiles)
	return s
}

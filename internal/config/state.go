// Package config owns launcher snapshots, instance allocation and local settings.
// It does not authenticate accounts, run tasks or touch game files.
package config

import (
	"slices"

	"github.com/CAUCraft/LXMCL/internal/account"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/server"
)

// State is a mutable configuration snapshot. Clone before editing a snapshot held
// by another operation; selection and revision checks never perform file I/O.
type State struct {
	MicrosoftProfile string                   `json:"microsoft_profile,omitempty"`
	StationKey       string                   `json:"skin_station_key,omitempty"`
	StationProfiles  []account.StationProfile `json:"skin_station_profiles,omitempty"`
	Servers          []server.Server          `json:"servers,omitempty"`
	Settings         instance.Settings        `json:"settings"`
	Instances        []instance.Instance      `json:"instances"`
	OfflinePlayers   []string                 `json:"offline_players,omitempty"`
	Selected         int                      `json:"selected"`
	NextID           int                      `json:"next_id"`
}

func (s State) Clone() State {
	s.Instances = slices.Clone(s.Instances)
	s.OfflinePlayers = slices.Clone(s.OfflinePlayers)
	s.Servers = slices.Clone(s.Servers)
	s.StationProfiles = slices.Clone(s.StationProfiles)
	return s
}

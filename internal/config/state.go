// Package config owns launcher snapshots, instance allocation and local settings.
// It does not authenticate accounts, run tasks or touch game files.
package config

import (
	"maps"
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
	InstanceNotes    map[int]string           `json:"instance_notes,omitempty"`
	InstanceIcons    map[int]string           `json:"instance_icons,omitempty"`
	PinnedInstances  map[int]bool             `json:"pinned_instances,omitempty"`
	InstanceGroups   map[int]string           `json:"instance_groups,omitempty"`
	LibrarySort      string                   `json:"library_sort,omitempty"`
	LibraryGroup     *string                  `json:"library_group,omitempty"` // nil shows all groups, empty selects ungrouped instances.
	OfflinePlayers   []string                 `json:"offline_players,omitempty"`
	Selected         int                      `json:"selected"`
	NextID           int                      `json:"next_id"`

	// Modrinth project/version pairs skipped by each instance.
	SkippedModUpdates map[int]map[string]bool `json:"skipped_mod_updates,omitempty"`
}

func (s State) Clone() State {
	s.Instances = slices.Clone(s.Instances)
	s.InstanceNotes = maps.Clone(s.InstanceNotes)
	s.PinnedInstances = maps.Clone(s.PinnedInstances)
	s.InstanceGroups = maps.Clone(s.InstanceGroups)
	s.SkippedModUpdates = maps.Clone(s.SkippedModUpdates)
	for id, versions := range s.SkippedModUpdates {
		s.SkippedModUpdates[id] = maps.Clone(versions)
	}
	s.InstanceIcons = maps.Clone(s.InstanceIcons)
	if s.LibraryGroup != nil {
		group := *s.LibraryGroup
		s.LibraryGroup = &group
	}
	s.OfflinePlayers = slices.Clone(s.OfflinePlayers)
	s.Servers = slices.Clone(s.Servers)
	s.StationProfiles = slices.Clone(s.StationProfiles)
	return s
}

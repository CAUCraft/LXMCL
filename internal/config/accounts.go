package config

import (
	"slices"

	"github.com/CAUCraft/LXMCL/internal/account"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

func (s State) Players() []string {
	names := slices.Clone(s.OfflinePlayers)
	if instance.ValidPlayerName(s.Settings.Player) && !slices.Contains(names, s.Settings.Player) {
		names = append(names, s.Settings.Player)
	}
	return names
}

func (s *State) UseOfflinePlayer(name string) error {
	if !instance.ValidPlayerName(name) {
		return i18n.Error("account.offline_name_invalid")
	}
	s.OfflinePlayers = s.Players()
	if !slices.Contains(s.OfflinePlayers, name) {
		s.OfflinePlayers = append(s.OfflinePlayers, name)
	}
	s.Settings.Player = name
	s.MicrosoftProfile, s.StationKey = "", ""
	return nil
}

func (s State) AccountKey() string {
	if s.StationKey != "" {
		return "station:" + s.StationKey
	}
	if s.MicrosoftProfile != "" {
		return "msa:" + s.MicrosoftProfile
	}
	return "offline:" + s.Settings.Player
}

func (s *State) ForgetOfflinePlayer(name string) {
	if name != s.Settings.Player {
		s.OfflinePlayers = slices.DeleteFunc(s.OfflinePlayers, func(player string) bool { return player == name })
	}
}

func (s State) Station(key string) (account.StationProfile, bool) {
	for _, p := range s.StationProfiles {
		if p.Key == key {
			return p, true
		}
	}
	return account.StationProfile{}, false
}

func (s *State) UseStation(p account.StationProfile) {
	s.MicrosoftProfile, s.StationKey = "", p.Key
	for i, existing := range s.StationProfiles {
		if existing.Key == p.Key {
			s.StationProfiles[i] = p
			return
		}
	}
	s.StationProfiles = append(s.StationProfiles, p)
}

func (s *State) ForgetStation(key string) {
	for i, p := range s.StationProfiles {
		if p.Key == key {
			s.StationProfiles = append(s.StationProfiles[:i], s.StationProfiles[i+1:]...)
			break
		}
	}
	if s.StationKey == key {
		s.StationKey = ""
	}
}

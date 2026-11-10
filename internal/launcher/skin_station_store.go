package launcher

import (
	"encoding/json"
	"errors"
	"io"
	"os"
)

type stationRecord struct {
	Profile StationProfile `json:"profile"`
	Access  string         `json:"access_token"`
	Client  string         `json:"client_token"`
	User    string         `json:"user_key"`
}

func (s Store) LoadStationSession(profile StationProfile) (StationSession, error) {
	if !MicrosoftCredentialsPersist() {
		return StationSession{}, ErrSkinStationLogin
	}
	path, err := stationVaultPath(s.Root, profile.Key)
	if err != nil {
		return StationSession{}, err
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return StationSession{}, ErrSkinStationLogin
	}
	if err != nil {
		return StationSession{}, err
	}
	defer f.Close()
	encrypted, err := io.ReadAll(io.LimitReader(f, maxVaultSize+1))
	if err != nil {
		return StationSession{}, err
	}
	if len(encrypted) == 0 || len(encrypted) > maxVaultSize {
		return StationSession{}, ErrSkinStationLogin
	}
	data, err := windowsCredentials(encrypted, false)
	if err != nil {
		return StationSession{}, ErrSkinStationLogin
	}
	defer clear(data)
	var record stationRecord
	if json.Unmarshal(data, &record) != nil {
		return StationSession{}, ErrSkinStationLogin
	}
	session := StationSession{record.Profile, record.Access, record.Client, record.User}
	if !session.validFor(profile) {
		return StationSession{}, ErrSkinStationLogin
	}
	return session, nil
}

func (s Store) SaveStationSession(session StationSession) error {
	if !session.validFor(session.Profile) {
		return ErrSkinStationLogin
	}
	if !MicrosoftCredentialsPersist() {
		return nil
	}
	path, err := stationVaultPath(s.Root, session.Profile.Key)
	if err != nil {
		return err
	}
	data, err := json.Marshal(stationRecord{session.Profile, session.access, session.client, session.user})
	if err != nil {
		return err
	}
	defer clear(data)
	encrypted, err := windowsCredentials(data, true)
	if err != nil {
		return errors.New("无法加密皮肤站凭据")
	}
	if len(encrypted) > maxVaultSize {
		return errors.New("皮肤站凭据过大")
	}
	return writeFile(path, encrypted)
}

func (s Store) DeleteStationSession(key string) error {
	if !MicrosoftCredentialsPersist() {
		return nil
	}
	path, err := stationVaultPath(s.Root, key)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s State) Station(key string) (StationProfile, bool) {
	for _, p := range s.StationProfiles {
		if p.Key == key {
			return p, true
		}
	}
	return StationProfile{}, false
}

func (s *State) UseStation(p StationProfile) {
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

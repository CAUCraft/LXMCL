package account

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

type stationRecord struct {
	Profile StationProfile `json:"profile"`
	Access  string         `json:"access_token"`
	Client  string         `json:"client_token"`
	User    string         `json:"user_key"`
}

func (s Vault) LoadStationSession(profile StationProfile) (StationSession, error) {
	if !CredentialsPersist() {
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

func (s Vault) SaveStationSession(session StationSession) error {
	if !session.validFor(session.Profile) {
		return ErrSkinStationLogin
	}
	if !CredentialsPersist() {
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
		return i18n.Error("station.vault_encrypt_failed")
	}
	if len(encrypted) > maxVaultSize {
		return i18n.Error("station.vault_too_large")
	}
	return storage.WriteFile(path, encrypted)
}

func (s Vault) DeleteStationSession(key string) error {
	if !CredentialsPersist() {
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

func stationVaultPath(root, key string) (string, error) {
	if b, err := hex.DecodeString(key); err != nil || len(b) != sha256.Size {
		return "", ErrSkinStationLogin
	}
	return filepath.Join(root, "skin-stations", strings.ToLower(key)+".dat"), nil
}

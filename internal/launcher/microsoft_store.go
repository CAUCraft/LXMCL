package launcher

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

const microsoftVault = "microsoft-account.dat"
const maxVaultSize = 64 << 10

func MicrosoftCredentialsPersist() bool { return runtime.GOOS == "windows" }

type microsoftRecord struct {
	Profile MicrosoftProfile `json:"profile"`
	Refresh string           `json:"refresh_token"`
}

func (s Store) LoadMicrosoftAccount() (MicrosoftAccount, error) {
	if !MicrosoftCredentialsPersist() {
		return MicrosoftAccount{}, nil
	}
	f, err := os.Open(filepath.Join(s.Root, microsoftVault))
	if errors.Is(err, os.ErrNotExist) {
		return MicrosoftAccount{}, nil
	}
	if err != nil {
		return MicrosoftAccount{}, err
	}
	defer f.Close()
	encrypted, err := io.ReadAll(io.LimitReader(f, maxVaultSize+1))
	if err != nil {
		return MicrosoftAccount{}, err
	}
	if len(encrypted) == 0 || len(encrypted) > maxVaultSize {
		return MicrosoftAccount{}, errors.New("Microsoft 凭据文件无效")
	}
	data, err := windowsCredentials(encrypted, false)
	if err != nil {
		return MicrosoftAccount{}, errors.New("无法解密本机 Microsoft 凭据")
	}
	defer clear(data)
	var record microsoftRecord
	if err := json.Unmarshal(data, &record); err != nil || !profileID.MatchString(record.Profile.ID) ||
		!playerID.MatchString(record.Profile.Name) || record.Refresh == "" {
		return MicrosoftAccount{}, errors.New("Microsoft 凭据已损坏，请重新登录")
	}
	return MicrosoftAccount{Profile: record.Profile, refreshToken: record.Refresh}, nil
}

func (s Store) SaveMicrosoftAccount(account MicrosoftAccount) error {
	if !MicrosoftCredentialsPersist() {
		return nil
	}
	if account.refreshToken == "" || !profileID.MatchString(account.Profile.ID) || !playerID.MatchString(account.Profile.Name) {
		return ErrMicrosoftRequired
	}
	plain, err := json.Marshal(microsoftRecord{Profile: account.Profile, Refresh: account.refreshToken})
	if err != nil {
		return err
	}
	defer clear(plain)
	encrypted, err := windowsCredentials(plain, true)
	if err != nil {
		return errors.New("无法加密本机 Microsoft 凭据")
	}
	return writeFile(filepath.Join(s.Root, microsoftVault), encrypted)
}

func (s Store) DeleteMicrosoftAccount() error {
	if !MicrosoftCredentialsPersist() {
		return nil
	}
	err := os.Remove(filepath.Join(s.Root, microsoftVault))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

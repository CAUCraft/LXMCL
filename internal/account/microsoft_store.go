package account

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

const microsoftVault = "microsoft-account.dat"

type microsoftRecord struct {
	Profile MicrosoftProfile `json:"profile"`
	Refresh string           `json:"refresh_token"`
}

func (s Vault) LoadMicrosoftAccount() (MicrosoftAccount, error) {
	if !CredentialsPersist() {
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
		return MicrosoftAccount{}, i18n.Error("microsoft.vault_invalid")
	}
	data, err := windowsCredentials(encrypted, false)
	if err != nil {
		return MicrosoftAccount{}, i18n.Error("microsoft.vault_decrypt_failed")
	}
	defer clear(data)
	var record microsoftRecord
	if err := json.Unmarshal(data, &record); err != nil || !profileID.MatchString(record.Profile.ID) ||
		!instance.ValidPlayerName(record.Profile.Name) || record.Refresh == "" {
		return MicrosoftAccount{}, i18n.Error("microsoft.vault_corrupt")
	}
	return MicrosoftAccount{Profile: record.Profile, refreshToken: record.Refresh}, nil
}

func (s Vault) SaveMicrosoftAccount(account MicrosoftAccount) error {
	if account.refreshToken == "" || !profileID.MatchString(account.Profile.ID) || !instance.ValidPlayerName(account.Profile.Name) {
		return ErrMicrosoftRequired
	}
	if !CredentialsPersist() {
		return nil
	}
	plain, err := json.Marshal(microsoftRecord{Profile: account.Profile, Refresh: account.refreshToken})
	if err != nil {
		return err
	}
	defer clear(plain)
	encrypted, err := windowsCredentials(plain, true)
	if err != nil {
		return i18n.Error("microsoft.vault_encrypt_failed")
	}
	return storage.WriteFile(filepath.Join(s.Root, microsoftVault), encrypted)
}

func (s Vault) DeleteMicrosoftAccount() error {
	if !CredentialsPersist() {
		return nil
	}
	err := os.Remove(filepath.Join(s.Root, microsoftVault))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

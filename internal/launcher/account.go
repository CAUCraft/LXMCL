package launcher

import (
	"context"
	"image"

	"github.com/CAUCraft/LXMCL/internal/account"
)

// Compatibility names keep desktop callers independent of the package migration.
type MicrosoftProfile = account.MicrosoftProfile
type MicrosoftAccount = account.MicrosoftAccount
type MinecraftSession = account.MinecraftSession
type MicrosoftDeviceCode = account.MicrosoftDeviceCode
type StationRole = account.StationRole
type StationProfile = account.StationProfile
type StationSession = account.StationSession
type StationLogin = account.StationLogin
type StationTextures = account.StationTextures
type ProfileTexture = account.ProfileTexture
type Appearance = account.Appearance

var (
	ErrMicrosoftRequired = account.ErrMicrosoftRequired
	ErrMicrosoftExpired  = account.ErrMicrosoftExpired
	ErrSkinStationLogin  = account.ErrSkinStationLogin
)

func BeginMicrosoftLogin(ctx context.Context) (MicrosoftDeviceCode, error) {
	return account.BeginMicrosoftLogin(ctx)
}

func CompleteMicrosoftLogin(ctx context.Context, code MicrosoftDeviceCode) (MicrosoftAccount, MinecraftSession, error) {
	return account.CompleteMicrosoftLogin(ctx, code)
}

func RefreshMicrosoftLogin(ctx context.Context, a MicrosoftAccount) (MicrosoftAccount, MinecraftSession, error) {
	return account.RefreshMicrosoftLogin(ctx, a)
}

func BeginStationLogin(ctx context.Context, endpoint, login, password string) (StationLogin, error) {
	return account.BeginStationLogin(ctx, endpoint, login, password)
}

func MicrosoftCredentialsPersist() bool { return account.CredentialsPersist() }

func FetchProfileTexture(ctx context.Context, raw string, skin bool) (image.Image, error) {
	return account.FetchProfileTexture(ctx, raw, skin)
}

func ReadSkinFile(path string) ([]byte, image.Image, error) { return account.ReadSkinFile(path) }

func (s Store) LoadMicrosoftAccount() (MicrosoftAccount, error) {
	return (account.Vault{Root: s.Root}).LoadMicrosoftAccount()
}

func (s Store) SaveMicrosoftAccount(a MicrosoftAccount) error {
	return (account.Vault{Root: s.Root}).SaveMicrosoftAccount(a)
}

func (s Store) DeleteMicrosoftAccount() error {
	return (account.Vault{Root: s.Root}).DeleteMicrosoftAccount()
}

func (s Store) LoadStationSession(p StationProfile) (StationSession, error) {
	return (account.Vault{Root: s.Root}).LoadStationSession(p)
}

func (s Store) SaveStationSession(a StationSession) error {
	return (account.Vault{Root: s.Root}).SaveStationSession(a)
}

func (s Store) DeleteStationSession(key string) error {
	return (account.Vault{Root: s.Root}).DeleteStationSession(key)
}

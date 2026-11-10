package account

import (
	"crypto/md5"
	"encoding/hex"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

// LaunchIdentity holds only the identity needed by a game process. Credentials stay
// private and are exposed exclusively through the launch argument substitution map.
type LaunchIdentity struct{ name, uuid, token, clientID, xuid, kind string }

func (s MinecraftSession) LaunchIdentity() (LaunchIdentity, error) {
	if !s.ValidFor(s.Profile.ID) || !profileID.MatchString(s.Profile.ID) || !instance.ValidPlayerName(s.Profile.Name) {
		return LaunchIdentity{}, ErrMicrosoftRequired
	}
	return LaunchIdentity{s.Profile.Name, s.Profile.ID, s.accessToken, microsoftClientID, s.xuid, "msa"}, nil
}

func (s StationSession) LaunchIdentity() (LaunchIdentity, error) {
	if !s.validFor(s.Profile) {
		return LaunchIdentity{}, ErrSkinStationLogin
	}
	return LaunchIdentity{name: s.Profile.Name, uuid: s.Profile.ID, token: s.access, kind: "mojang"}, nil
}

func OfflineIdentity(name string) (LaunchIdentity, error) {
	if !instance.ValidPlayerName(name) {
		return LaunchIdentity{}, i18n.Error("account.offline_name_invalid")
	}
	uuid := md5.Sum([]byte("OfflinePlayer:" + name))
	uuid[6] = uuid[6]&0x0f | 0x30
	uuid[8] = uuid[8]&0x3f | 0x80
	return LaunchIdentity{name: name, uuid: hex.EncodeToString(uuid[:]), token: "0", kind: "legacy"}, nil
}

// Arguments returns a fresh map containing secret process arguments. Do not log it.
func (i LaunchIdentity) Arguments() map[string]string {
	return map[string]string{"auth_player_name": i.name, "auth_uuid": i.uuid, "auth_access_token": i.token, "clientid": i.clientID, "auth_xuid": i.xuid, "user_type": i.kind}
}

func (i LaunchIdentity) String() string { return i.kind + ":" + i.uuid }

func (i LaunchIdentity) GoString() string { return i.String() }

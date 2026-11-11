package account

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"slices"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

var ErrSkinStationLogin = i18n.Error("station.login_required")

type StationRole struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type StationProfile struct {
	Key  string `json:"key"`
	API  string `json:"api"`
	Site string `json:"site"`
	StationRole
}

func (p StationProfile) Homepage() string {
	root, rootErr := stationRoot(p.API)
	site, siteErr := stationURL(p.Site)
	if rootErr != nil || siteErr != nil || !strings.EqualFold(root.Host, site.Host) {
		return ""
	}
	return site.String()
}

type StationSession struct {
	Profile              StationProfile
	access, client, user string
}

type stationAuth struct {
	AccessToken       string
	ClientToken       string
	SelectedProfile   StationRole
	AvailableProfiles []StationRole
	User              struct{ ID string }
}

type StationLogin struct {
	api, site, client, loginKey string
	auth                        stationAuth
}

func BeginStationLogin(ctx context.Context, endpoint, login, password string) (StationLogin, error) {
	return (Client{}).BeginStationLogin(ctx, endpoint, login, password)
}

func (c Client) BeginStationLogin(ctx context.Context, endpoint, login, password string) (StationLogin, error) {
	u, err := stationRoot(endpoint)
	if err != nil {
		return StationLogin{}, err
	}
	login = strings.TrimSpace(login)
	if login == "" || password == "" {
		return StationLogin{}, i18n.Error("station.credentials_required")
	}
	root, meta, _, err := c.stationMetadata(ctx, u, true)
	if err != nil {
		return StationLogin{}, err
	}
	site := (&url.URL{Scheme: root.Scheme, Host: root.Host}).String()
	if home, err := stationURL(meta.Meta.Links.Homepage); err == nil && home.Host == root.Host {
		site = home.String()
	}
	var client [16]byte
	if _, err := rand.Read(client[:]); err != nil {
		return StationLogin{}, err
	}
	p := StationLogin{api: root.String(), site: site, client: hex.EncodeToString(client[:]), loginKey: stationHash(strings.ToLower(login))}
	body := map[string]any{"username": login, "password": password, "clientToken": p.client, "requestUser": true,
		"agent": map[string]any{"name": "Minecraft", "version": 1}}
	if err := c.post(ctx, i18n.Text("station.login_service"), p.api+"authserver/authenticate", body, &p.auth); err != nil {
		var failure *authResponseError
		if errors.As(err, &failure) && (failure.status == 401 || failure.status == 403) {
			return StationLogin{}, i18n.Error("station.login_denied")
		}
		return StationLogin{}, err
	}
	if !p.auth.valid(p.client) {
		return StationLogin{}, i18n.Error("station.login_credentials_invalid")
	}
	if len(p.Roles()) == 0 {
		return StationLogin{}, i18n.Error("station.roles_missing")
	}
	return p, nil
}

func (a stationAuth) valid(client string) bool {
	return client != "" && a.ClientToken == client && a.AccessToken != "" && len(a.AccessToken) <= 4096 &&
		!strings.ContainsAny(a.AccessToken, "\r\n\x00")
}

func validStationRole(r StationRole) bool {
	return profileID.MatchString(r.ID) && instance.ValidPlayerName(r.Name)
}

func (p StationLogin) Roles() []StationRole {
	if p.auth.SelectedProfile.ID != "" {
		if validStationRole(p.auth.SelectedProfile) {
			return []StationRole{p.auth.SelectedProfile}
		}
		return nil
	}
	var roles []StationRole
	for _, role := range p.auth.AvailableProfiles {
		if validStationRole(role) && !slices.ContainsFunc(roles, func(r StationRole) bool { return strings.EqualFold(r.ID, role.ID) }) {
			roles = append(roles, role)
		}
	}
	return roles
}

func (p StationLogin) Bind(ctx context.Context, id string) (StationSession, error) {
	return (Client{}).BindStationRole(ctx, p, id)
}

func (c Client) BindStationRole(ctx context.Context, p StationLogin, id string) (StationSession, error) {
	roles := p.Roles()
	i := slices.IndexFunc(roles, func(r StationRole) bool { return strings.EqualFold(r.ID, id) })
	if i < 0 {
		return StationSession{}, i18n.Error("station.role_required")
	}
	reply := p.auth
	if reply.SelectedProfile.ID == "" {
		var err error
		reply, err = c.refreshStation(ctx, p.api, p.auth.AccessToken, p.client, &roles[i])
		if err != nil {
			return StationSession{}, err
		}
	}
	if !reply.valid(p.client) || !validStationRole(reply.SelectedProfile) || !strings.EqualFold(reply.SelectedProfile.ID, id) ||
		(p.auth.User.ID != "" && reply.User.ID != "" && p.auth.User.ID != reply.User.ID) {
		return StationSession{}, i18n.Error("station.account_or_role_changed")
	}
	user := p.auth.User.ID
	if user == "" {
		user = reply.User.ID
	}
	userKey := "login:" + p.loginKey
	if user != "" {
		userKey = "id:" + stationHash(user)
	}
	role := reply.SelectedProfile
	role.ID = strings.ToLower(role.ID)
	profile := StationProfile{API: p.api, Site: p.site, StationRole: role, Key: stationKey(p.api, userKey, role.ID)}
	return StationSession{Profile: profile, access: reply.AccessToken, client: p.client, user: userKey}, nil
}

func stationHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func stationKey(api, user, role string) string {
	return stationHash(api + "\x00" + user + "\x00" + role)
}

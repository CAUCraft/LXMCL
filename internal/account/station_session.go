package account

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

func (s StationSession) validFor(p StationProfile) bool {
	root, err := stationRoot(p.API)
	return err == nil && root.String() == p.API && validStationRole(p.StationRole) &&
		s.Profile.Key == p.Key && s.Profile.API == p.API && s.Profile.ID == p.ID &&
		p.Key == stationKey(p.API, s.user, p.ID) && s.access != "" && s.client != ""
}

func (c Client) refreshStation(ctx context.Context, api, token, client string, role *StationRole) (stationAuth, error) {
	var reply stationAuth
	body := map[string]any{"accessToken": token, "clientToken": client, "requestUser": true}
	if role != nil {
		body["selectedProfile"] = role
	}
	err := c.post(ctx, i18n.Text("station.session_service"), api+"authserver/refresh", body, &reply)
	return reply, err
}

func (s StationSession) Verify(ctx context.Context) (StationSession, error) {
	return (Client{}).VerifyStation(ctx, s)
}

func (c Client) VerifyStation(ctx context.Context, s StationSession) (StationSession, error) {
	if !s.validFor(s.Profile) {
		return s, ErrSkinStationLogin
	}
	body, _ := json.Marshal(map[string]string{"accessToken": s.access, "clientToken": s.client})
	err := c.request(ctx, i18n.Text("station.session_service"), s.Profile.API+"authserver/validate", http.MethodPost, "application/json", bytes.NewReader(body), "", nil)
	if err == nil {
		return s, nil
	}
	var failure *authResponseError
	if !errors.As(err, &failure) || (failure.status != 401 && failure.status != 403) {
		return s, err
	}
	reply, err := c.refreshStation(ctx, s.Profile.API, s.access, s.client, nil)
	if err != nil {
		if errors.As(err, &failure) && (failure.status == 401 || failure.status == 403) {
			return s, ErrSkinStationLogin
		}
		return s, err
	}
	if !reply.valid(s.client) || !validStationRole(reply.SelectedProfile) ||
		!strings.EqualFold(reply.SelectedProfile.ID, s.Profile.ID) ||
		(reply.User.ID != "" && strings.HasPrefix(s.user, "id:") && s.user != "id:"+stationHash(reply.User.ID)) {
		return s, ErrSkinStationLogin
	}
	s.access, s.Profile.Name = reply.AccessToken, reply.SelectedProfile.Name
	return s, nil
}

package launcher

import (
	"context"
	"time"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

func authorizeLaunch(ctx context.Context, store Store, r LaunchRequest, report func(Progress)) (LaunchResult, error) {
	onlineID, account, session := r.State.MicrosoftProfile, r.Account, r.Session
	stationKey := r.State.StationKey
	station, _ := r.State.Station(stationKey)
	stationSession, stationState := r.StationSession, r.State
	var err error
	var refreshed, stationStateChanged bool
	if stationKey != "" {
		report(Progress{Stage: i18n.Text("progress.verify_station")})
		if station.Key == "" {
			err = ErrSkinStationLogin
		} else if stationSession.Profile.Key != stationKey {
			stationSession, err = store.LoadStationSession(station)
		}
		if err == nil {
			original := stationSession
			stationSession, err = original.Verify(ctx)
			if err == nil && original != stationSession {
				err = store.SaveStationSession(stationSession)
				if err != nil {
					stationSession = original
				}
			}
			if err == nil && stationSession.Profile.Name != station.Name {
				stationState = stationState.Clone()
				stationState.UseStation(stationSession.Profile)
				err = store.Save(stationState)
				stationStateChanged = err == nil
			}
		}
	} else if onlineID != "" {
		if !session.ValidFor(onlineID) {
			report(Progress{Stage: i18n.Text("progress.verify_microsoft")})
			if account.Profile.ID != onlineID {
				err = ErrMicrosoftRequired
			} else {
				authCtx, stop := context.WithTimeout(ctx, 65*time.Second)
				var nextAccount MicrosoftAccount
				nextAccount, session, err = RefreshMicrosoftLogin(authCtx, account)
				stop()
				if err == nil {
					account, refreshed = nextAccount, true
					if saveErr := store.SaveMicrosoftAccount(account); saveErr != nil {
						err = i18n.Error("microsoft.refresh_save_failed", saveErr)
						session = MinecraftSession{}
					}
				}
			}
		}
	}
	return LaunchResult{Account: account, Session: session, StationSession: stationSession, State: stationState, CredentialsChanged: refreshed, StateChanged: stationStateChanged}, err
}

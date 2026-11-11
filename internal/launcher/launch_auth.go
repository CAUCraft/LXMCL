package launcher

import (
	"context"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

func authorizeLaunch(ctx context.Context, store Store, r LaunchRequest, report func(Progress)) (LaunchResult, error) {
	result := LaunchResult{Account: r.Account, Session: r.Session, StationSession: r.StationSession, State: r.State}
	if r.State.StationKey != "" {
		report(Progress{Stage: i18n.Text("progress.verify_station")})
		auth, err := store.AuthorizeStation(ctx, r.State, r.StationSession)
		result.StationSession, result.State = auth.Session, auth.State
		result.CredentialsSaved, result.StateChanged = auth.CredentialsSaved, auth.StateSaved
		return result, err
	}
	if id := r.State.MicrosoftProfile; id != "" {
		report(Progress{Stage: i18n.Text("progress.verify_microsoft")})
		auth, err := store.AuthorizeMicrosoft(ctx, id, r.Account, r.Session)
		result.Account, result.Session = auth.Account, auth.Session
		result.CredentialsChanged = auth.Account != r.Account || auth.Session != r.Session
		result.CredentialsSaved = auth.CredentialsSaved
		return result, err
	}
	return result, nil
}

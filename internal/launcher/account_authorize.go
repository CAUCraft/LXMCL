package launcher

import (
	"context"
	"errors"
	"time"
)

// MicrosoftAuthorization retains the latest credentials even on failure. The
// caller must check the error before using Session. Every authorization retries
// credential storage, including when the cached game session is still valid.
type MicrosoftAuthorization struct {
	Account          MicrosoftAccount
	Session          MinecraftSession
	CredentialsSaved bool
}

func (s Store) AuthorizeMicrosoft(ctx context.Context, id string, a MicrosoftAccount, session MinecraftSession) (MicrosoftAuthorization, error) {
	result := MicrosoftAuthorization{Account: a, Session: session}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if id == "" || a.Profile.ID != id {
		result.Session = MinecraftSession{}
		return result, ErrMicrosoftRequired
	}
	var authErr error
	if !session.ValidFor(id) {
		authCtx, stop := context.WithTimeout(ctx, 65*time.Second)
		result.Account, result.Session, authErr = RefreshMicrosoftLogin(authCtx, a)
		stop()
	}
	// OAuth may rotate credentials before a later service fails or ctx expires.
	// Preserve that rotation before returning the authentication failure.
	var saveErr error
	result.CredentialsSaved, saveErr = saveAccountCredentials(func() error {
		return s.SaveMicrosoftAccount(result.Account)
	})
	return result, errors.Join(authErr, saveErr)
}

type StationAuthorization struct {
	Session StationSession
	AccountCommit
}

// AuthorizeStation verifies the selected identity, saves the latest session,
// then updates its displayed profile. A failed write never restores an old token.
func (s Store) AuthorizeStation(ctx context.Context, state State, session StationSession) (StationAuthorization, error) {
	result := StationAuthorization{Session: session, AccountCommit: AccountCommit{State: state}}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	profile, ok := state.Station(state.StationKey)
	if !ok {
		return result, ErrSkinStationLogin
	}
	var err error
	if session.Profile.Key != profile.Key {
		result.Session, err = s.LoadStationSession(profile)
		if err != nil {
			return result, err
		}
	}
	var authErr, saveErr error
	result.Session, authErr = result.Session.Verify(ctx)
	// The input may contain an earlier rotation whose storage failed. Even a
	// transient verification failure must not skip retrying that pending save.
	result.CredentialsSaved, saveErr = saveAccountCredentials(func() error { return s.SaveStationSession(result.Session) })
	if err := errors.Join(authErr, saveErr); err != nil {
		return result, err
	}
	if result.Session.Profile != profile {
		next := state.Clone()
		next.UseStation(result.Session.Profile)
		result.AccountCommit, err = s.saveAccountSelection(result.AccountCommit, next)
	}
	return result, err
}

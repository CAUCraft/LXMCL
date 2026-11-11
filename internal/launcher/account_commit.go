package launcher

import (
	"github.com/CAUCraft/LXMCL/internal/account"
	"github.com/CAUCraft/LXMCL/internal/i18n"
)

// AccountCommit describes completed writes, including partial failures. State
// remains the input snapshot until configuration is saved. CredentialsSaved is
// false on platforms without a vault; CredentialsDeleted also covers no file.
// Callers serialize these operations with other configuration writes.
type AccountCommit struct {
	State              State
	CredentialsSaved   bool
	CredentialsDeleted bool
	StateSaved         bool
}

// CommitMicrosoftLogin saves verified credentials before selecting the account.
// A failed selection save never rolls back credentials that may have rotated.
func (s Store) CommitMicrosoftLogin(state State, a MicrosoftAccount) (AccountCommit, error) {
	next := state.Clone()
	next.MicrosoftProfile, next.StationKey = a.Profile.ID, ""
	return s.commitAccount(state, next, func() error { return s.SaveMicrosoftAccount(a) })
}

// CommitStationLogin accepts an already bound session. Retain it on failure and
// retry this operation without binding the role again.
func (s Store) CommitStationLogin(state State, session StationSession) (AccountCommit, error) {
	next := state.Clone()
	next.UseStation(session.Profile)
	return s.commitAccount(state, next, func() error { return s.SaveStationSession(session) })
}

func (s Store) commitAccount(state, next State, credentials func() error) (AccountCommit, error) {
	result := AccountCommit{State: state}
	var err error
	result.CredentialsSaved, err = saveAccountCredentials(credentials)
	if err != nil {
		return result, err
	}
	return s.saveAccountSelection(result, next)
}

func saveAccountCredentials(save func() error) (bool, error) {
	if err := save(); err != nil {
		return false, i18n.Error("account.credentials_save_failed", err)
	}
	return account.CredentialsPersist(), nil
}

func (s Store) saveAccountSelection(result AccountCommit, next State) (AccountCommit, error) {
	if err := s.Save(next); err != nil {
		return result, i18n.Error("account.selection_save_failed", err)
	}
	result.State, result.StateSaved = next, true
	return result, nil
}

// RemoveMicrosoftAccount deletes credentials before clearing the selection.
// Clear in-memory credentials when CredentialsDeleted is true, even on error.
func (s Store) RemoveMicrosoftAccount(state State) (AccountCommit, error) {
	next := state.Clone()
	next.MicrosoftProfile = ""
	return s.removeAccount(state, next, s.DeleteMicrosoftAccount)
}

// RemoveStationAccount keeps the profile available for retry if saving the
// removal fails after the credentials have already been deleted.
func (s Store) RemoveStationAccount(state State, key string) (AccountCommit, error) {
	next := state.Clone()
	next.ForgetStation(key)
	return s.removeAccount(state, next, func() error { return s.DeleteStationSession(key) })
}

func (s Store) removeAccount(state, next State, credentials func() error) (AccountCommit, error) {
	result := AccountCommit{State: state}
	if err := credentials(); err != nil {
		return result, i18n.Error("account.credentials_delete_failed", err)
	}
	result.CredentialsDeleted = true
	return s.saveAccountSelection(result, next)
}

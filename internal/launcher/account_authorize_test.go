package launcher

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/account"
)

func TestLaunchAuthorizationRetainsRotationAndRetriesStorage(t *testing.T) {
	base, original, _, _ := accountProtocolFixture(t)
	networkErr := errors.New("Xbox unavailable")
	mode, expectedRefresh := "rotate", "refresh-fixture"
	s := Store{Root: t.TempDir(), Accounts: account.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/consumers/oauth2/v2.0/token":
			if err := r.ParseForm(); err != nil || r.Form.Get("refresh_token") != expectedRefresh {
				t.Fatal("authorization did not retain the latest refresh credential")
			}
			if mode == "offline" {
				return nil, networkErr
			}
			return fixtureResponse(http.StatusOK, []byte(`{"access_token":"oauth-fixture","refresh_token":"rotated-fixture"}`)), nil
		case "/user/authenticate":
			if mode == "rotate" {
				return nil, networkErr
			}
		}
		return base.Transport.RoundTrip(r)
	})}}
	state, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := state.Create("submitted", "1.21.1"); err != nil {
		t.Fatal(err)
	}
	state.MicrosoftProfile = original.Profile.ID
	i, _ := state.Find(state.Selected)
	path := filepath.Join(s.Root, "microsoft-account.dat")
	if account.CredentialsPersist() {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	game := Game{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("game installation continued after failed authorization")
		return nil, nil
	})}
	result, err := game.Execute(t.Context(), s, LaunchRequest{Instance: i, State: state, Account: original}, nil)
	if !errors.Is(err, networkErr) || !result.AuthFailed || !result.CredentialsChanged || result.CredentialsSaved ||
		result.Account == original || result.Account.Profile != original.Profile || result.Session != (MinecraftSession{}) || result.Running.PID != 0 {
		t.Fatalf("failed launch lost authorization outcome: %v", err)
	}
	if account.CredentialsPersist() {
		var fileErr *os.LinkError
		if !errors.As(err, &fileErr) {
			t.Fatal("combined authorization error lost the credential storage failure")
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	mode, expectedRefresh = "offline", "rotated-fixture"
	retry, err := s.AuthorizeMicrosoft(t.Context(), original.Profile.ID, result.Account, result.Session)
	if !errors.Is(err, networkErr) || retry.CredentialsSaved != account.CredentialsPersist() || retry.Account != result.Account {
		t.Fatalf("network failure skipped pending credential storage: %v", err)
	}
	if account.CredentialsPersist() {
		stored, err := s.LoadMicrosoftAccount()
		if err != nil || stored != result.Account {
			t.Fatal("latest rotated credential was not saved")
		}
	}
	mode = "success"
	retry, err = s.AuthorizeMicrosoft(t.Context(), original.Profile.ID, retry.Account, retry.Session)
	if err != nil || !retry.Session.ValidFor(original.Profile.ID) {
		t.Fatalf("authorization retry could not use the rotated credential: %v", err)
	}
}

func TestStationAuthorizationKeepsPendingSessionAndCommitsProfile(t *testing.T) {
	_, _, _, original := accountProtocolFixture(t)
	networkErr := errors.New("station unavailable")
	mode := "rotate"
	s := Store{Root: t.TempDir(), Accounts: account.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if mode == "offline" {
			return nil, networkErr
		}
		var request struct{ AccessToken, ClientToken string }
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		switch r.URL.Path {
		case "/authserver/validate":
			if mode == "rotate" {
				return fixtureResponse(http.StatusForbidden, []byte(`{}`)), nil
			}
			if request.AccessToken != "rotated-station" {
				t.Fatal("authorization restored an obsolete station session")
			}
			return fixtureResponse(http.StatusNoContent, nil), nil
		case "/authserver/refresh":
			body, _ := json.Marshal(map[string]any{"accessToken": "rotated-station", "clientToken": request.ClientToken,
				"selectedProfile": map[string]string{"id": original.Profile.ID, "name": "UpdatedPlayer"}})
			return fixtureResponse(http.StatusOK, body), nil
		default:
			t.Fatalf("unexpected station request: %s", r.URL.Path)
			return nil, nil
		}
	})}}
	state, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	state.UseStation(original.Profile)
	before := state.Clone()
	path := filepath.Join(s.Root, "skin-stations", original.Profile.Key+".dat")
	if account.CredentialsPersist() {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	} else {
		// Without persistent credentials, fail the following configuration write.
		if err := os.Mkdir(filepath.Join(s.Root, "launcher.json"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	result, err := s.AuthorizeStation(t.Context(), state, original)
	if err == nil || result.StateSaved || result.Session == original || result.Session.Profile.Name != "UpdatedPlayer" ||
		!reflect.DeepEqual(result.State, before) || !reflect.DeepEqual(state, before) {
		t.Fatal("failed commit lost the rotated session or changed the selected profile")
	}
	mode = "offline"
	failed, err := s.AuthorizeStation(t.Context(), state, result.Session)
	if !errors.Is(err, networkErr) || failed.Session != result.Session || failed.StateSaved {
		t.Fatal("failed verification lost the pending session")
	}
	if account.CredentialsPersist() {
		var fileErr *os.LinkError
		if !errors.As(err, &fileErr) {
			t.Fatal("combined station error lost the credential storage failure")
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	} else if err := os.Remove(filepath.Join(s.Root, "launcher.json")); err != nil {
		t.Fatal(err)
	}
	retry, err := s.AuthorizeStation(t.Context(), state, failed.Session)
	if !errors.Is(err, networkErr) || retry.CredentialsSaved != account.CredentialsPersist() || retry.StateSaved {
		t.Fatal("network failure skipped saving the pending station session")
	}
	if account.CredentialsPersist() {
		stored, err := s.LoadStationSession(original.Profile)
		if err != nil || stored != result.Session {
			t.Fatal("pending station session was not persisted")
		}
	}
	mode = "success"
	retry, err = s.AuthorizeStation(t.Context(), state, retry.Session)
	if err != nil || !retry.StateSaved || retry.Session.Profile.Name != "UpdatedPlayer" {
		t.Fatalf("station retry did not commit the updated profile: %v", err)
	}
	stored, err := s.Load()
	if err != nil || !reflect.DeepEqual(stored, retry.State) || !reflect.DeepEqual(state, before) {
		t.Fatal("station authorization did not preserve snapshot ownership")
	}
}

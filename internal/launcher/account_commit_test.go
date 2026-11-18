package launcher

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/CAUCraft/LXMCL/internal/account"
)

type accountCommitFixture struct {
	name, path string
	commit     func(Store, State) (AccountCommit, error)
	remove     func(Store, State) (AccountCommit, error)
	cached     func(*testing.T, Store)
}

// Obtain real opaque credentials through the protocol client; no token fields
// or persistence hooks are exposed solely for these application tests.
func accountProtocolFixture(t *testing.T) (account.Client, MicrosoftAccount, MinecraftSession, StationSession) {
	t.Helper()
	const id = "0123456789abcdef0123456789abcdef"
	client := account.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		var body string
		switch r.URL.Path {
		case "/consumers/oauth2/v2.0/token":
			body = `{"access_token":"oauth-fixture","refresh_token":"refresh-fixture"}`
		case "/user/authenticate":
			body = `{"Token":"xbox-fixture"}`
		case "/xsts/authorize":
			body = `{"Token":"xsts-fixture","DisplayClaims":{"xui":[{"uhs":"42"}]}}`
		case "/authentication/login_with_xbox":
			body = `{"access_token":"game-fixture","expires_in":3600}`
		case "/entitlements/mcstore":
			body = `{"items":[{"name":"product_minecraft"}]}`
		case "/minecraft/profile":
			body = `{"id":"` + id + `","name":"Player"}`
		case "/":
			body = `{}`
		case "/authserver/authenticate":
			var request struct{ ClientToken string }
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			data, _ := json.Marshal(map[string]any{"accessToken": "station-fixture", "clientToken": request.ClientToken,
				"selectedProfile": map[string]string{"id": id, "name": "StationPlayer"}})
			body = string(data)
		default:
			t.Fatalf("unexpected authentication request: %s", r.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	a, session, err := client.CompleteMicrosoftLogin(t.Context(), MicrosoftDeviceCode{ExpiresAt: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	login, err := client.BeginStationLogin(t.Context(), "https://skin.example.test/", "user", "fixture-password")
	if err != nil {
		t.Fatal(err)
	}
	station, err := client.BindStationRole(t.Context(), login, id)
	if err != nil {
		t.Fatal(err)
	}
	return client, a, session, station
}

func accountCommitFixtures(t *testing.T) []accountCommitFixture {
	t.Helper()
	_, a, session, station := accountProtocolFixture(t)
	return []accountCommitFixture{
		{name: "microsoft", path: "microsoft-account.dat",
			commit: func(s Store, state State) (AccountCommit, error) { return s.CommitMicrosoftLogin(state, a) },
			remove: func(s Store, state State) (AccountCommit, error) { return s.RemoveMicrosoftAccount(state) },
			cached: func(t *testing.T, s Store) {
				result, err := s.AuthorizeMicrosoft(t.Context(), a.Profile.ID, a, session)
				if err == nil || result.CredentialsSaved || result.Account != a || result.Session != session {
					t.Fatal("cached authorization bypassed failed storage or discarded the latest credentials")
				}
			}},
		{name: "station", path: filepath.Join("skin-stations", station.Profile.Key+".dat"),
			commit: func(s Store, state State) (AccountCommit, error) { return s.CommitStationLogin(state, station) },
			remove: func(s Store, state State) (AccountCommit, error) {
				return s.RemoveStationAccount(state, station.Profile.Key)
			}},
	}
}

func TestAccountCommitRetainsCompletedStagesForRetry(t *testing.T) {
	for _, fixture := range accountCommitFixtures(t) {
		t.Run(fixture.name, func(t *testing.T) {
			s := Store{Root: t.TempDir()}
			state, err := s.Load()
			if err != nil {
				t.Fatal(err)
			}
			original := state.Clone()
			credentialPath := filepath.Join(s.Root, fixture.path)
			if account.CredentialsPersist() {
				if err := os.MkdirAll(credentialPath, 0700); err != nil {
					t.Fatal(err)
				}
				result, err := fixture.commit(s, state)
				if err == nil || errors.Unwrap(err) == nil || result.CredentialsSaved || result.StateSaved || !reflect.DeepEqual(result.State, state) {
					t.Fatal("credential failure changed configuration or lost the error cause")
				}
				if fixture.cached != nil {
					fixture.cached(t, s)
				}
				if err := os.Remove(credentialPath); err != nil {
					t.Fatal(err)
				}
			}
			configPath := filepath.Join(s.Root, "launcher.json")
			if err := os.Mkdir(configPath, 0700); err != nil {
				t.Fatal(err)
			}
			result, err := fixture.commit(s, state)
			if err == nil || errors.Unwrap(err) == nil || result.CredentialsSaved != account.CredentialsPersist() || result.StateSaved || !reflect.DeepEqual(result.State, state) {
				t.Fatal("configuration failure did not report completed credential storage")
			}
			if account.CredentialsPersist() {
				if info, err := os.Stat(credentialPath); err != nil || !info.Mode().IsRegular() {
					t.Fatal("configuration failure discarded saved credentials")
				}
			}
			if err := os.Remove(configPath); err != nil {
				t.Fatal(err)
			}
			result, err = fixture.commit(s, state)
			if err != nil || !result.StateSaved || !reflect.DeepEqual(state, original) {
				t.Fatalf("save retry failed or mutated the input snapshot: %v", err)
			}
			loaded, err := s.Load()
			if err != nil || !reflect.DeepEqual(loaded, result.State) {
				t.Fatal("reported configuration differs from the saved selection")
			}
		})
	}
}

func TestAccountRemovalRetainsSelectionUntilSaved(t *testing.T) {
	for _, fixture := range accountCommitFixtures(t) {
		t.Run(fixture.name, func(t *testing.T) {
			s := Store{Root: t.TempDir()}
			state, err := s.Load()
			if err != nil {
				t.Fatal(err)
			}
			signedIn, err := fixture.commit(s, state)
			if err != nil {
				t.Fatal(err)
			}
			configPath := filepath.Join(s.Root, "launcher.json")
			if err := os.Remove(configPath); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(configPath, 0700); err != nil {
				t.Fatal(err)
			}
			result, err := fixture.remove(s, signedIn.State)
			if err == nil || !result.CredentialsDeleted || result.StateSaved || !reflect.DeepEqual(result.State, signedIn.State) {
				t.Fatal("partial removal lost the retry target or hid credential deletion")
			}
			if _, err := os.Stat(filepath.Join(s.Root, fixture.path)); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("credential deletion was not completed")
			}
			if err := os.Remove(configPath); err != nil {
				t.Fatal(err)
			}
			result, err = fixture.remove(s, result.State)
			if err != nil || !result.CredentialsDeleted || !result.StateSaved || result.State.AccountKey() != state.AccountKey() {
				t.Fatalf("removal retry failed: %v", err)
			}
		})
	}
}

package account

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

func TestMicrosoftDeviceLoginAndIdentity(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef"
	var stages []string
	polls := 0
	owned := true
	profile := id
	client := Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		res := func(status int, body string) (*http.Response, error) {
			return fixtureResponse(status, []byte(body)), nil
		}
		path := r.URL.Path
		stages = append(stages, path)
		switch path {
		case "/consumers/oauth2/v2.0/devicecode":
			if err := r.ParseForm(); err != nil || r.Form.Get("client_id") != microsoftClientID || r.Form.Get("scope") != microsoftScope {
				t.Fatalf("wrong device-code request: %v", err)
			}
			return res(200, `{"user_code":"CODE-123","device_code":"device-fixture","verification_uri":"https://www.microsoft.com/devicelogin","expires_in":900,"interval":5}`)
		case "/consumers/oauth2/v2.0/token":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.Form.Get("grant_type") == "refresh_token" {
				if r.Form.Get("refresh_token") != "refresh-fixture" {
					t.Fatal("refresh did not use the saved account")
				}
				return res(200, `{"access_token":"oauth-fixture","refresh_token":"rotated-fixture"}`)
			}
			if r.Form.Get("device_code") != "device-fixture" {
				t.Fatal("incorrect OAuth polling challenge")
			}
			polls++
			if polls == 1 {
				return res(400, `{"error":"authorization_pending"}`)
			}
			return res(200, `{"access_token":"oauth-fixture","refresh_token":"refresh-fixture"}`)
		case "/user/authenticate":
			var body struct{ Properties map[string]string }
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Properties["RpsTicket"] != "d=oauth-fixture" {
				t.Fatalf("OAuth token was not exchanged for Xbox Live: %v", err)
			}
			return res(200, `{"Token":"xbl-fixture"}`)
		case "/xsts/authorize":
			var body struct {
				RelyingParty string
				Properties   struct{ UserTokens []string }
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RelyingParty != "rp://api.minecraftservices.com/" ||
				!slices.Equal(body.Properties.UserTokens, []string{"xbl-fixture"}) {
				t.Fatalf("invalid XSTS request: %v", err)
			}
			return res(200, `{"Token":"xsts-fixture","DisplayClaims":{"xui":[{"uhs":"42","xid":"9001"}]}}`)
		case "/authentication/login_with_xbox":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["identityToken"] != "XBL3.0 x=42;xsts-fixture" {
				t.Fatalf("invalid Minecraft login request: %v", err)
			}
			return res(200, `{"access_token":"minecraft-fixture","expires_in":86400}`)
		case "/entitlements/mcstore":
			if r.Header.Get("Authorization") != "Bearer minecraft-fixture" {
				t.Fatal("Minecraft ownership was not verified with the game token")
			}
			if !owned {
				return res(200, `{"items":[]}`)
			}
			return res(200, `{"items":[{"name":"product_minecraft"}]}`)
		case "/minecraft/profile":
			return res(200, `{"id":"`+profile+`","name":"OnlinePlayer"}`)
		default:
			t.Fatalf("unexpected authentication request: %s", r.URL)
			return nil, nil
		}
	})}
	code, err := client.BeginMicrosoftLogin(t.Context())
	if err != nil || code.Code != "CODE-123" || code.interval != 5*time.Second {
		t.Fatalf("device authorization failed: %v", err)
	}
	code.interval = time.Millisecond
	account, session, err := client.CompleteMicrosoftLogin(t.Context(), code)
	if err != nil || polls != 2 || len(stages) != 8 || account.Profile.ID != id || account.refreshToken != "refresh-fixture" {
		t.Fatalf("authentication did not reach the verified profile: %v", err)
	}
	identity, err := session.LaunchIdentity()
	if err != nil || identity.name != "OnlinePlayer" || identity.uuid != id || identity.token != "minecraft-fixture" ||
		identity.xuid != "9001" || identity.clientID != microsoftClientID || identity.kind != "msa" {
		t.Fatalf("launch did not use the Microsoft profile: %v", err)
	}
	session.expiresAt = time.Now().Add(-time.Second)
	if _, err := session.LaunchIdentity(); !errors.Is(err, ErrMicrosoftRequired) {
		t.Fatal("expired Microsoft session fell back to an offline account")
	}
	if refreshed, _, err := client.RefreshMicrosoftLogin(t.Context(), account); err != nil || refreshed.refreshToken != "rotated-fixture" {
		t.Fatalf("refresh did not rotate the credential: %v", err)
	}
	profile = strings.Repeat("a", 32)
	if _, _, err := client.RefreshMicrosoftLogin(t.Context(), account); err == nil {
		t.Fatal("refresh accepted a different Minecraft identity")
	}
	owned = false
	if _, _, err := client.RefreshMicrosoftLogin(t.Context(), account); err == nil || !strings.Contains(err.Error(), i18n.Text("microsoft.minecraft_license_missing")) ||
		stages[len(stages)-1] != "/entitlements/mcstore" {
		t.Fatalf("account without game ownership was accepted: %v", err)
	}
}

func TestMicrosoftDeviceRejectsUntrustedVerificationLink(t *testing.T) {
	client := Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Hostname() != "login.microsoftonline.com" {
			t.Fatal("device code request used an unexpected host")
		}
		return fixtureResponse(200, []byte(`{"user_code":"CODE","device_code":"device","verification_uri":"https://www.microsoft.com.example.test/login","expires_in":900}`)), nil
	})}
	if _, err := client.BeginMicrosoftLogin(t.Context()); err == nil {
		t.Fatal("accepted a spoofed device login site")
	}
}

func TestMicrosoftCredentialVault(t *testing.T) {
	store := Vault{Root: t.TempDir()}
	account := MicrosoftAccount{Profile: MicrosoftProfile{ID: strings.Repeat("a", 32), Name: "Player"}, refreshToken: "fake-refresh-token-for-test"}
	if err := store.SaveMicrosoftAccount(account); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.Root, microsoftVault)
	blob, err := os.ReadFile(path)
	if !CredentialsPersist() {
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatal("platform without a vault wrote credentials")
		}
		return
	}
	if err != nil || bytes.Contains(blob, []byte(account.refreshToken)) || bytes.Contains(blob, []byte(account.Profile.ID)) {
		t.Fatalf("credential vault did not encrypt the account: %v", err)
	}
	if got, err := store.LoadMicrosoftAccount(); err != nil || got != account {
		t.Fatalf("failed to restore encrypted account: %v", err)
	}
	if err := store.DeleteMicrosoftAccount(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("logout left credentials on disk: %v", err)
	}
}

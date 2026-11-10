package launcher

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestStationDiscoveryLoginRefreshAndTextures(t *testing.T) {
	const (
		root = "https://skin.example.test/yggdrasil/"
		id1  = "00112233445566778899aabbccddeeff"
		id2  = "112233445566778899aabbccddeeff00"
	)
	roles := []StationRole{{id1, "First"}, {id2, "Second"}}
	skin := textureFixture(t, 64, 64)
	cape := textureFixture(t, 64, 32)
	var clientToken string
	var requests []string
	verified := false
	c := authClient{&http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		requests = append(requests, r.URL.Path)
		if r.Header.Get("Authorization") != "" || r.URL.Scheme != "https" {
			t.Fatalf("unexpected request: %s", r.URL)
		}
		reply := func(status int, value any) *http.Response {
			data, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			return fixtureResponse(status, data)
		}
		var body map[string]json.RawMessage
		if r.Method == http.MethodPost {
			if r.URL.Host != "skin.example.test" || json.NewDecoder(r.Body).Decode(&body) != nil {
				t.Fatal("credentials were not sent only to the configured HTTPS API")
			}
		}
		switch r.URL.Path {
		case "/":
			res := reply(200, "")
			res.Header = http.Header{"X-Authlib-Injector-Api-Location": {"/yggdrasil"}}
			return res, nil
		case "/yggdrasil/":
			return reply(200, map[string]any{"skinDomains": []string{"textures.example.test"}}), nil
		case "/yggdrasil/authserver/authenticate":
			if string(body["username"]) != `"user-fixture"` || string(body["password"]) != `"secret-fixture"` ||
				string(body["requestUser"]) != "true" || json.Unmarshal(body["clientToken"], &clientToken) != nil || clientToken == "" {
				t.Fatal("incorrect authenticate request")
			}
			return reply(200, map[string]any{"accessToken": "unbound", "clientToken": clientToken,
				"availableProfiles": roles, "user": map[string]string{"id": "provider-user"}}), nil
		case "/yggdrasil/authserver/refresh":
			token := "bound"
			if !verified {
				var role StationRole
				if json.Unmarshal(body["selectedProfile"], &role) != nil || role != roles[1] || string(body["accessToken"]) != `"unbound"` {
					t.Fatal("refresh did not bind the selected role")
				}
			} else {
				if body["selectedProfile"] != nil || string(body["accessToken"]) != `"bound"` {
					t.Fatal("launch refresh changed the bound role")
				}
				token = "rotated"
			}
			return reply(200, map[string]any{"accessToken": token, "clientToken": clientToken,
				"selectedProfile": roles[1], "user": map[string]string{"id": "provider-user"}}), nil
		case "/yggdrasil/authserver/validate":
			verified = true
			return reply(403, map[string]string{"error": "ForbiddenOperationException"}), nil
		case "/yggdrasil/sessionserver/session/minecraft/profile/" + id2:
			raw := `{"profileId":"` + id2 + `","textures":{"SKIN":{"url":"https://textures.example.test/skin","metadata":{"model":"slim"}},"CAPE":{"url":"https://textures.example.test/cape"}}}`
			return reply(200, map[string]any{"id": id2, "properties": []map[string]string{{"name": "textures", "value": base64.StdEncoding.EncodeToString([]byte(raw))}}}), nil
		case "/skin":
			return fixtureResponse(200, skin), nil
		case "/cape":
			return fixtureResponse(200, cape), nil
		}
		t.Fatalf("unexpected request: %s", r.URL)
		return nil, nil
	})}}
	pending, err := c.beginStationLogin(t.Context(), "https://skin.example.test", "user-fixture", "secret-fixture")
	if err != nil || !slices.Equal(pending.Roles(), roles) {
		t.Fatalf("login did not return the two roles: %v", err)
	}
	if _, err := c.bindStationRole(t.Context(), pending, "invalid"); err == nil {
		t.Fatal("accepted an unknown role")
	}
	session, err := c.bindStationRole(t.Context(), pending, id2)
	if err != nil || session.Profile.API != root || session.Profile.Name != "Second" {
		t.Fatalf("could not bind the selected role: %v", err)
	}
	session, err = c.verifyStation(t.Context(), session)
	identity, identityErr := stationIdentity(session)
	if err != nil || identityErr != nil || identity.uuid != id2 || identity.kind != "mojang" || identity.token != "rotated" {
		t.Fatalf("launch identity did not use the verified station role: %v %v", err, identityErr)
	}
	tex, err := c.stationTextures(t.Context(), session.Profile)
	if err != nil || tex.Skin == nil || tex.Cape == nil || !tex.Slim {
		t.Fatalf("skin and cape were not decoded: %v", err)
	}
	want := []string{"/", "/yggdrasil/", "/yggdrasil/authserver/authenticate", "/yggdrasil/authserver/refresh",
		"/yggdrasil/authserver/validate", "/yggdrasil/authserver/refresh", "/yggdrasil/",
		"/yggdrasil/sessionserver/session/minecraft/profile/" + id2, "/skin", "/cape"}
	if !slices.Equal(want, requests) {
		t.Fatalf("unexpected protocol flow: %v", requests)
	}
	state := State{MicrosoftProfile: id1}
	state.UseStation(session.Profile)
	if state.AccountKey() != "station:"+session.Profile.Key || state.MicrosoftProfile != "" {
		t.Fatal("station selection did not replace Microsoft as the launch identity")
	}
	store := Store{Root: t.TempDir()}
	if err := store.Save(state); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveStationSession(session); err != nil {
		t.Fatal(err)
	}
	if MicrosoftCredentialsPersist() {
		file, _ := stationVaultPath(store.Root, session.Profile.Key)
		data, err := os.ReadFile(file)
		if err != nil || bytes.Contains(data, []byte(session.access)) || bytes.Contains(data, []byte(session.client)) {
			t.Fatal("station credentials were not encrypted")
		}
		got, err := store.LoadStationSession(session.Profile)
		if err != nil || got != session {
			t.Fatal("station session could not be restored")
		}
	}
	if err := state.UseOfflinePlayer("Player"); err != nil || state.StationKey != "" {
		t.Fatal("offline launch retained the station identity")
	}
}

func TestStationDoesNotSendCredentialsToDiscoveredDifferentHost(t *testing.T) {
	c := authClient{&http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet || r.URL.Host != "skin.example.test" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL)
		}
		res := fixtureResponse(200, nil)
		res.Header = http.Header{"X-Authlib-Injector-Api-Location": {"https://other.example.test/auth/"}}
		return res, nil
	})}}
	if _, err := c.beginStationLogin(t.Context(), "https://skin.example.test", "login-fixture", "secret-fixture"); err == nil {
		t.Fatal("accepted cross-host discovery for a credential endpoint")
	}
}

func TestStationAgentRejectsUnverifiedDownload(t *testing.T) {
	c := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != stationAgentURL {
			t.Fatal("agent did not use the pinned release")
		}
		return fixtureResponse(200, bytes.Repeat([]byte{1}, stationAgentSize)), nil
	})}
	if _, err := stationAgent(t.Context(), t.TempDir(), c); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatal("unverified agent could be used for launch")
	}
	if _, err := stationIdentity(StationSession{}); !errors.Is(err, ErrSkinStationLogin) {
		t.Fatal("missing station credentials silently fell back to an offline identity")
	}
}

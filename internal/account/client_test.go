package account

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestClientDoesNotFollowCredentialRedirects(t *testing.T) {
	session := MinecraftSession{
		Profile:     MicrosoftProfile{ID: strings.Repeat("a", 32), Name: "Player"},
		accessToken: "redirect-test-token", expiresAt: time.Now().Add(time.Hour),
	}
	requests := 0
	client := Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.URL.Host != "api.minecraftservices.com" || r.Header.Get("Authorization") != "Bearer redirect-test-token" {
			t.Fatal("credentials sent to an unexpected destination")
		}
		response := fixtureResponse(http.StatusTemporaryRedirect, nil)
		response.Header.Set("Location", "https://other.example.test/profile")
		return response, nil
	})}
	if _, err := client.Appearance(t.Context(), session); err == nil || requests != 1 {
		t.Fatalf("redirect accepted: %d requests, %v", requests, err)
	}
}

func TestLaunchIdentityExposesCredentialsOnlyAsArguments(t *testing.T) {
	session := MinecraftSession{
		Profile:     MicrosoftProfile{ID: strings.Repeat("a", 32), Name: "Player"},
		accessToken: "identity-test-token", expiresAt: time.Now().Add(time.Hour), xuid: "42",
	}
	identity, err := session.LaunchIdentity()
	if err != nil {
		t.Fatal(err)
	}
	args := identity.Arguments()
	if args["auth_access_token"] != session.accessToken || args["auth_player_name"] != "Player" || args["auth_xuid"] != "42" || args["user_type"] != "msa" {
		t.Fatal("launch identity lost the authenticated profile")
	}
	encoded, err := json.Marshal(identity)
	if err != nil || strings.Contains(string(encoded), session.accessToken) || strings.Contains(fmt.Sprintf("%v %+v %#v", identity, identity, identity), session.accessToken) {
		t.Fatal("identity serialization exposed a token")
	}
	args["auth_access_token"] = "changed"
	if identity.Arguments()["auth_access_token"] != session.accessToken {
		t.Fatal("argument map changed the captured identity")
	}
}

package account

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestVaultReadsExistingCredentialFiles(t *testing.T) {
	if !CredentialsPersist() {
		t.Skip("Windows user-encrypted credential fixtures")
	}
	vault := Vault{Root: t.TempDir()}
	writeLegacy := func(path, data string) {
		t.Helper()
		blob, err := windowsCredentials([]byte(data), true)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, blob, 0600); err != nil {
			t.Fatal(err)
		}
	}
	const id = "00112233445566778899aabbccddeeff"
	writeLegacy(filepath.Join(vault.Root, "microsoft-account.dat"), `{"profile":{"id":"`+id+`","name":"Player"},"refresh_token":"legacy-refresh"}`)
	account, err := vault.LoadMicrosoftAccount()
	if err != nil || account.Profile.ID != id || account.refreshToken != "legacy-refresh" {
		t.Fatalf("old Microsoft vault no longer loads: %v", err)
	}
	profile := StationProfile{API: "https://skin.example.test/yggdrasil/", Site: "https://skin.example.test", StationRole: StationRole{ID: id, Name: "Player"}}
	const user = "id:legacy-user"
	profile.Key = stationKey(profile.API, user, id)
	encoded, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(vault.Root, "skin-stations", profile.Key+".dat")
	writeLegacy(path, `{"profile":`+string(encoded)+`,"access_token":"legacy-access","client_token":"legacy-client","user_key":"`+user+`"}`)
	session, err := vault.LoadStationSession(profile)
	if err != nil || session.access != "legacy-access" || session.client != "legacy-client" {
		t.Fatalf("old station vault no longer loads: %v", err)
	}
	if err := vault.DeleteStationSession(profile.Key); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("station logout left the old credential file")
	}
}

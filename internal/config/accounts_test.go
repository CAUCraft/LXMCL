package config

import (
	"testing"

	"github.com/CAUCraft/LXMCL/internal/account"
)

func TestAccountSelectionSurvivesStatePersistence(t *testing.T) {
	profile := account.StationProfile{Key: "selected-station", StationRole: account.StationRole{ID: "00112233445566778899aabbccddeeff", Name: "Player"}}
	state := State{MicrosoftProfile: "previous-microsoft"}
	state.UseStation(profile)
	store := Store{Root: t.TempDir()}
	if err := store.Save(state); err != nil {
		t.Fatal(err)
	}
	restored, err := store.Load()
	if err != nil || restored.AccountKey() != "station:"+profile.Key || restored.MicrosoftProfile != "" {
		t.Fatalf("station selection changed: %v", err)
	}
	if err := restored.UseOfflinePlayer("OfflinePlayer"); err != nil || restored.StationKey != "" || restored.AccountKey() != "offline:OfflinePlayer" {
		t.Fatalf("offline selection retained an online identity: %v", err)
	}
}

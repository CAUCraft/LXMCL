package launcher

import (
	"errors"
	"testing"
)

func TestAccountSelectionSurvivesStatePersistence(t *testing.T) {
	profile := StationProfile{Key: "selected-station", StationRole: StationRole{ID: "00112233445566778899aabbccddeeff", Name: "Player"}}
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

func TestInvalidAccountCannotFallBackOffline(t *testing.T) {
	if _, err := launchIdentity("Player", &MinecraftSession{}); !errors.Is(err, ErrMicrosoftRequired) {
		t.Fatal("invalid Microsoft session fell back to offline identity")
	}
	if _, err := stationIdentity(StationSession{}); !errors.Is(err, ErrSkinStationLogin) {
		t.Fatal("invalid station session fell back to offline identity")
	}
}

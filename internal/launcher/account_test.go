package launcher

import (
	"errors"
	"testing"
)

func TestInvalidAccountCannotFallBackOffline(t *testing.T) {
	if _, err := launchIdentity("Player", &MinecraftSession{}); !errors.Is(err, ErrMicrosoftRequired) {
		t.Fatal("invalid Microsoft session fell back to offline identity")
	}
	if _, err := stationIdentity(StationSession{}); !errors.Is(err, ErrSkinStationLogin) {
		t.Fatal("invalid station session fell back to offline identity")
	}
}

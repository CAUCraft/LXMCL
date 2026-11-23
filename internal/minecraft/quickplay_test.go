package minecraft

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/instance"
)

func TestQuickPlayArguments(t *testing.T) {
	address := "127.0.0.1:25565"
	var m metadata
	if err := json.Unmarshal([]byte(`[{"rules":[{"action":"allow","features":{"is_quick_play_multiplayer":true}}],"value":["--quickPlayMultiplayer","${quickPlayMultiplayer}"]}]`), &m.Arguments.Game); err != nil {
		t.Fatal(err)
	}
	args, err := clientArgs(m, map[string]string{}, instance.LaunchOptions{}, address, "")
	if err != nil || !slices.Equal(args, []string{"--quickPlayMultiplayer", address}) {
		t.Fatalf("quick play = %v, %v", args, err)
	}
	world := "My World 世界"
	if _, err := clientArgs(m, map[string]string{}, instance.LaunchOptions{}, "", world); err == nil {
		t.Fatal("missing singleplayer declaration accepted")
	}
	m.Arguments.Game = append(m.Arguments.Game, json.RawMessage(`{"rules":[{"action":"allow","features":{"is_quick_play_singleplayer":true}}],"value":["--quickPlaySingleplayer","${quickPlaySingleplayer}"]}`))
	args, err = clientArgs(m, map[string]string{}, instance.LaunchOptions{}, "", world)
	if err != nil || !slices.Equal(args, []string{"--quickPlaySingleplayer", world}) {
		t.Fatal(args, err)
	}
	args, err = clientArgs(m, map[string]string{}, instance.LaunchOptions{}, "", "")
	if err != nil || len(args) != 0 {
		t.Fatal("ordinary launch gained a destination", args, err)
	}
}

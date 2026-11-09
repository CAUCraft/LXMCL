package launcher

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"slices"
	"testing"
	"time"
)

func TestPingJavaServerAndBuildQuickPlayArguments(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	address := listener.Addr().String()
	reply := make(chan error, 1)
	go func() {
		c, err := listener.Accept()
		if err != nil {
			reply <- err
			return
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(4 * time.Second))
		r := bufio.NewReader(c)
		size, err := binary.ReadUvarint(r)
		if err != nil || size < 8 || size > 1024 {
			reply <- fmt.Errorf("invalid handshake: %d, %v", size, err)
			return
		}
		if _, err := io.CopyN(io.Discard, r, int64(size)); err != nil {
			reply <- err
			return
		}
		request := make([]byte, 2)
		if _, err := io.ReadFull(r, request); err != nil || !slices.Equal(request, []byte{1, 0}) {
			reply <- fmt.Errorf("invalid status request: %v, %v", request, err)
			return
		}
		data := []byte(`{"version":{"name":"1.21.1"},"players":{"online":3,"max":12}}`)
		packet := append(binary.AppendUvarint([]byte{0}, uint64(len(data))), data...)
		_, err = c.Write(append(binary.AppendUvarint(nil, uint64(len(packet))), packet...))
		reply <- err
	}()
	status, err := PingServer(t.Context(), address)
	if err != nil || status.Version != "1.21.1" || status.Online != 3 || status.Max != 12 {
		t.Fatalf("status = %+v, %v", status, err)
	}
	if err := <-reply; err != nil {
		t.Fatal(err)
	}
	var m metadata
	if err := json.Unmarshal([]byte(`[{"rules":[{"action":"allow","features":{"is_quick_play_multiplayer":true}}],"value":["--quickPlayMultiplayer","${quickPlayMultiplayer}"]}]`), &m.Arguments.Game); err != nil {
		t.Fatal(err)
	}
	args, err := clientArgs(m, map[string]string{}, LaunchOptions{}, address)
	if err != nil || !slices.Equal(args, []string{"--quickPlayMultiplayer", address}) {
		t.Fatalf("quick play = %v, %v", args, err)
	}
}

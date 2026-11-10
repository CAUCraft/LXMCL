package server_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/CAUCraft/LXMCL/internal/server"
)

func TestClientKeepsSRVHandshakeAndCancelsIO(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	conn, peer := net.Pipe()
	defer conn.Close()
	defer peer.Close()
	deadline := time.AfterFunc(5*time.Second, func() { _ = peer.Close() })
	defer deadline.Stop()
	client := server.Client{
		LookupSRV: func(_ context.Context, service, proto, name string) (string, []*net.SRV, error) {
			if service != "minecraft" || proto != "tcp" || name != "play.example" {
				t.Errorf("unexpected SRV lookup: %s %s %s", service, proto, name)
			}
			return "", []*net.SRV{{Target: "node.example.", Port: 24444}}, nil
		},
		DialContext: func(_ context.Context, network, target string) (net.Conn, error) {
			if network != "tcp" || target != "node.example:24444" {
				t.Errorf("unexpected connection: %s %s", network, target)
			}
			return conn, nil
		},
	}
	reply := make(chan error, 1)
	go func() {
		// Protocol -1, original host and port 25565, followed by the status request.
		want := append([]byte{22, 0, 255, 255, 255, 255, 15, 12}, []byte("play.example")...)
		want = append(want, 0x63, 0xdd, 1, 1, 0)
		got := make([]byte, len(want))
		_, err := io.ReadFull(peer, got)
		if err == nil && !bytes.Equal(got, want) {
			err = fmt.Errorf("unexpected handshake: %x", got)
		}
		cancel() // Leave the status read blocked; cancellation must close the connection.
		reply <- err
	}()
	if _, err := client.Ping(ctx, "Play.Example:25565"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled query returned %v", err)
	}
	if !deadline.Stop() {
		t.Fatal("query did not stop after cancellation")
	}
	if err := <-reply; err != nil {
		t.Fatal(err)
	}
}

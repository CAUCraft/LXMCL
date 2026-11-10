package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

type Server struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

var serverHost = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9.-]{0,251}[a-zA-Z0-9])?$`)

func Address(address string) (string, error) {
	value := strings.TrimSpace(address)
	host, port, err := net.SplitHostPort(value)
	if err != nil {
		host, port = value, "25565"
		if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
			host = host[1 : len(host)-1]
		}
	}
	if ip, err := netip.ParseAddr(host); err == nil && ip.Zone() == "" {
		host = ip.String()
	} else if !serverHost.MatchString(host) || strings.Contains(host, "..") {
		return "", i18n.Error("server.address_invalid")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", i18n.Error("server.port_invalid")
	}
	return net.JoinHostPort(strings.ToLower(host), strconv.Itoa(n)), nil
}

type Status struct {
	Version     string
	Online, Max int
	Latency     time.Duration
	Icon        image.Image
}

// Ping requests Java server status without logging in.
func Ping(ctx context.Context, address string) (Status, error) {
	address, err := Address(address)
	if err != nil {
		return Status{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	host, port, _ := net.SplitHostPort(address)
	target := address
	if _, err := netip.ParseAddr(host); err != nil && port == "25565" {
		if _, records, err := net.DefaultResolver.LookupSRV(ctx, "minecraft", "tcp", host); err == nil && len(records) > 0 {
			target = net.JoinHostPort(strings.TrimSuffix(records[0].Target, "."), strconv.Itoa(int(records[0].Port)))
		}
	}
	start := time.Now()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", target)
	if err != nil {
		return Status{}, err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	n, _ := strconv.Atoi(port)
	packet := binary.AppendUvarint([]byte{0}, 0xffffffff) // protocol -1: status only
	packet = binary.AppendUvarint(packet, uint64(len(host)))
	packet = append(packet, host...)
	packet = binary.BigEndian.AppendUint16(packet, uint16(n))
	packet = append(packet, 1) // status state
	payload := binary.AppendUvarint(nil, uint64(len(packet)))
	if _, err := conn.Write(append(append(payload, packet...), 1, 0)); err != nil {
		return Status{}, err
	}
	r := bufio.NewReader(conn)
	size, err := binary.ReadUvarint(r)
	if err != nil || size < 2 || size > 1<<20 {
		return Status{}, i18n.Error("server.status_packet_invalid")
	}
	data := make([]byte, int(size))
	if _, err := io.ReadFull(r, data); err != nil {
		return Status{}, err
	}
	b := bytes.NewReader(data)
	packetID, err := binary.ReadUvarint(b)
	if err != nil || packetID != 0 {
		return Status{}, i18n.Error("server.status_missing")
	}
	length, err := binary.ReadUvarint(b)
	if err != nil || length != uint64(b.Len()) {
		return Status{}, i18n.Error("server.status_incomplete")
	}
	var status struct {
		Version struct{ Name string }
		Players struct{ Online, Max int }
		Favicon string
	}
	if err := json.NewDecoder(b).Decode(&status); err != nil {
		return Status{}, err
	}
	return Status{Version: status.Version.Name, Online: status.Players.Online, Max: status.Players.Max,
		Latency: time.Since(start), Icon: serverIcon(status.Favicon)}, nil
}

func serverIcon(raw string) image.Image {
	encoded, ok := strings.CutPrefix(raw, "data:image/png;base64,")
	if !ok || len(encoded) > 64<<10 {
		return nil
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil
	}
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width != 64 || config.Height != 64 {
		return nil
	}
	img, _ := png.Decode(bytes.NewReader(data))
	return img
}

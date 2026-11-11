package server

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

// queryStatus uses the original address in the handshake, even when DNS selects
// a different connection target. Connection lifetime belongs to Client.Ping.
func queryStatus(conn io.ReadWriter, host string, port uint16) (Status, error) {
	packet := binary.AppendUvarint([]byte{0}, 0xffffffff) // protocol -1: status only
	packet = binary.AppendUvarint(packet, uint64(len(host)))
	packet = append(packet, host...)
	packet = binary.BigEndian.AppendUint16(packet, port)
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
		Icon: serverIcon(status.Favicon)}, nil
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

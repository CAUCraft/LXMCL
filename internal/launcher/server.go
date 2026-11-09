package launcher

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/netip"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

type Server struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

var serverHost = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9.-]{0,251}[a-zA-Z0-9])?$`)

func ServerAddress(address string) (string, error) {
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
		return "", errors.New("请输入 Java 服务器域名或 IP，可附端口")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", errors.New("服务器端口须为 1–65535")
	}
	return net.JoinHostPort(strings.ToLower(host), strconv.Itoa(n)), nil
}

func (s *State) AddServer(name, address string) error {
	address, err := ServerAddress(address)
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = address
	}
	if len([]rune(name)) > 80 {
		return errors.New("服务器名称不能超过 80 个字符")
	}
	if slices.ContainsFunc(s.Servers, func(server Server) bool { return server.Address == address }) {
		return errors.New("已收藏此服务器")
	}
	s.Servers = append(s.Servers, Server{Name: name, Address: address})
	return nil
}

func (s *State) RemoveServer(address string) {
	s.Servers = slices.DeleteFunc(s.Servers, func(server Server) bool { return server.Address == address })
}

type ServerStatus struct {
	Version     string
	Online, Max int
	Latency     time.Duration
}

// PingServer requests Java server status without logging in.
func PingServer(ctx context.Context, address string) (ServerStatus, error) {
	address, err := ServerAddress(address)
	if err != nil {
		return ServerStatus{}, err
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
		return ServerStatus{}, err
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
		return ServerStatus{}, err
	}
	r := bufio.NewReader(conn)
	size, err := binary.ReadUvarint(r)
	if err != nil || size < 2 || size > 1<<20 {
		return ServerStatus{}, errors.New("服务器未返回有效的 Java 状态包")
	}
	data := make([]byte, int(size))
	if _, err := io.ReadFull(r, data); err != nil {
		return ServerStatus{}, err
	}
	b := bytes.NewReader(data)
	packetID, err := binary.ReadUvarint(b)
	if err != nil || packetID != 0 {
		return ServerStatus{}, errors.New("服务器未返回 Java 状态")
	}
	length, err := binary.ReadUvarint(b)
	if err != nil || length != uint64(b.Len()) {
		return ServerStatus{}, errors.New("服务器状态内容不完整")
	}
	var status struct {
		Version struct{ Name string }
		Players struct{ Online, Max int }
	}
	if err := json.NewDecoder(b).Decode(&status); err != nil {
		return ServerStatus{}, err
	}
	return ServerStatus{status.Version.Name, status.Players.Online, status.Players.Max, time.Since(start)}, nil
}

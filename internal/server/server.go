package server

import (
	"net"
	"net/netip"
	"regexp"
	"strconv"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

type Server struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

// NewFavorite validates and normalizes an entry for the launcher favorites list.
func NewFavorite(name, address string) (Server, error) {
	address, err := Address(address)
	if err != nil {
		return Server{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = address
	} else if name != address && len([]rune(name)) > 80 {
		return Server{}, i18n.Error("server.favorite_name_too_long")
	}
	return Server{Name: name, Address: address}, nil
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

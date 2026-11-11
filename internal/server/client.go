package server

import (
	"context"
	"image"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

type Status struct {
	Version     string
	Online, Max int
	Latency     time.Duration
	Icon        image.Image
}

// Client queries Java server status. Nil functions use the system resolver and
// dialer. Custom functions must honor context; Ping owns and closes connections.
type Client struct {
	LookupSRV   func(ctx context.Context, service, proto, name string) (string, []*net.SRV, error)
	DialContext func(ctx context.Context, network, address string) (net.Conn, error)
}

// Ping requests Java server status without logging in, using system networking.
func Ping(ctx context.Context, address string) (Status, error) {
	return (Client{}).Ping(ctx, address)
}

// Ping applies a five-second timeout to DNS, connection and the status exchange.
// Latency measures connection and response time, excluding the SRV lookup.
func (c Client) Ping(ctx context.Context, address string) (Status, error) {
	if err := ctx.Err(); err != nil {
		return Status{}, err
	}
	address, err := Address(address)
	if err != nil {
		return Status{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	lookup, dial := c.LookupSRV, c.DialContext
	if lookup == nil {
		lookup = net.DefaultResolver.LookupSRV
	}
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}
	host, port, _ := net.SplitHostPort(address)
	target := address
	if _, err := netip.ParseAddr(host); err != nil && port == "25565" {
		if _, records, err := lookup(ctx, "minecraft", "tcp", host); err == nil && len(records) > 0 {
			target = net.JoinHostPort(strings.TrimSuffix(records[0].Target, "."), strconv.Itoa(int(records[0].Port)))
		}
	}
	if err := ctx.Err(); err != nil {
		return Status{}, err
	}
	start := time.Now()
	conn, err := dial(ctx, "tcp", target)
	if err != nil {
		return Status{}, err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	n, _ := strconv.Atoi(port)
	status, err := queryStatus(conn, host, uint16(n))
	if err != nil {
		if ctx.Err() != nil {
			return Status{}, ctx.Err()
		}
		return Status{}, err
	}
	status.Latency = time.Since(start)
	return status, nil
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

// Package discovery provides mDNS service advertisement and browsing for
// zero-config fleet monitoring (OSS-022).
package discovery

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
)

const (
	serviceType    = "_keldron._tcp"
	domain         = "local."
	browseInterval = 30 * time.Second
)

// MDNSAdvertiser advertises the agent as a _keldron._tcp service on the local network.
type MDNSAdvertiser struct {
	server *zeroconf.Server
}

// NewAdvertiser registers a _keldron._tcp service with the given parameters.
// Returns nil and logs a warning on failure (graceful degradation).
func NewAdvertiser(deviceName string, port int, version string, deviceCount int) (*MDNSAdvertiser, error) {
	txt := []string{
		"version=" + version,
		"device_name=" + deviceName,
		"device_count=" + strconv.Itoa(deviceCount),
	}
	server, err := zeroconf.Register(deviceName, serviceType, domain, port, txt, nil)
	if err != nil {
		return nil, fmt.Errorf("mDNS register: %w", err)
	}
	return &MDNSAdvertiser{server: server}, nil
}

// Stop shuts down the mDNS advertisement.
func (a *MDNSAdvertiser) Stop() {
	if a != nil && a.server != nil {
		a.server.Shutdown()
	}
}

// MDNSBrowser browses for _keldron._tcp services and notifies on found/removed.
type MDNSBrowser struct {
	onFound   func(addr string, deviceName string)
	onRemoved func(addr string)
}

// NewBrowser creates a browser that calls onFound when a service appears and
// onRemoved when it disappears (detected via 30s re-browse diff).
func NewBrowser(onFound func(addr string, deviceName string), onRemoved func(addr string)) *MDNSBrowser {
	return &MDNSBrowser{
		onFound:   onFound,
		onRemoved: onRemoved,
	}
}

// Start browses for _keldron._tcp services until ctx is cancelled.
// Uses a 30s re-browse loop to detect disappeared services.
// Returns an error only if initial resolver creation fails.
func (b *MDNSBrowser) Start(ctx context.Context) error {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return fmt.Errorf("mDNS resolver: %w", err)
	}

	seen := make(map[string]string) // addr -> deviceName
	var mu sync.Mutex

	for {
		entries := make(chan *zeroconf.ServiceEntry)
		browseCtx, cancel := context.WithTimeout(ctx, browseInterval)

		go func() {
			defer close(entries)
			_ = resolver.Browse(browseCtx, serviceType, domain, entries)
		}()

		current := make(map[string]string)
		for entry := range entries {
			if entry == nil {
				continue
			}
			addr := entryToAddr(entry)
			if addr == "" {
				continue
			}
			deviceName := txtDeviceName(entry)
			current[addr] = deviceName
			if b.onFound != nil {
				b.onFound(addr, deviceName)
			}
		}
		cancel()

		// Diff: call onRemoved for addresses that disappeared
		mu.Lock()
		for addr := range seen {
			if _, ok := current[addr]; !ok && b.onRemoved != nil {
				b.onRemoved(addr)
			}
		}
		seen = current
		mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Continue to next browse cycle
		}
	}
}

func entryToAddr(entry *zeroconf.ServiceEntry) string {
	var host string
	if len(entry.AddrIPv4) > 0 {
		host = entry.AddrIPv4[0].String()
	} else if len(entry.AddrIPv6) > 0 {
		host = entry.AddrIPv6[0].String() // JoinHostPort adds brackets for IPv6
	} else if entry.HostName != "" {
		host = entry.HostName
	} else {
		return ""
	}
	return net.JoinHostPort(host, strconv.Itoa(entry.Port))
}

func txtDeviceName(entry *zeroconf.ServiceEntry) string {
	for _, s := range entry.Text {
		if strings.HasPrefix(s, "device_name=") {
			return strings.TrimPrefix(s, "device_name=")
		}
	}
	return entry.ServiceRecord.Instance
}

// IsLocalAddress returns true if the given host is a local interface address.
func IsLocalAddress(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP.Equal(ip) {
			return true
		}
	}
	return false
}

// IsSelf returns true if the discovered address/deviceName refers to this agent.
// Used by the hub to filter self-discovery.
func IsSelf(addr, deviceName, selfDeviceName string, selfPrometheusPort int) bool {
	if deviceName != "" && selfDeviceName != "" && deviceName == selfDeviceName {
		return true
	}
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port != selfPrometheusPort {
		return false
	}
	return IsLocalAddress(host)
}

// NewAdvertiserSafe creates an advertiser, logging a warning and returning nil on failure.
// Use for graceful degradation when mDNS is unavailable (e.g. corporate firewalls).
func NewAdvertiserSafe(deviceName string, port int, version string, deviceCount int, logger *slog.Logger) *MDNSAdvertiser {
	adv, err := NewAdvertiser(deviceName, port, version, deviceCount)
	if err != nil {
		if logger != nil {
			logger.Warn("mDNS advertisement unavailable — agent will not be discoverable", "error", err)
		}
		return nil
	}
	return adv
}

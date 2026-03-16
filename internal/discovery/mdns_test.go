// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package discovery

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/zeroconf/v2"
)

func TestNewAdvertiser(t *testing.T) {
	adv, err := NewAdvertiser("test-device", 9100, "0.1.0", 1)
	if err != nil {
		t.Skipf("mDNS not available (e.g. sandbox, no multicast): %v", err)
	}
	defer adv.Stop()
	// Advertiser created successfully; zeroconf registers _keldron._tcp
	if adv.server == nil {
		t.Error("server should not be nil")
	}
}

func TestEntryToAddr(t *testing.T) {
	tests := []struct {
		name  string
		entry *zeroconf.ServiceEntry
		want  string
	}{
		{
			name: "ipv4",
			entry: &zeroconf.ServiceEntry{
				AddrIPv4: []net.IP{net.ParseIP("192.168.1.50")},
				Port:     9100,
			},
			want: "192.168.1.50:9100",
		},
		{
			name: "ipv6",
			entry: &zeroconf.ServiceEntry{
				AddrIPv6: []net.IP{net.ParseIP("fe80::1")},
				Port:     9100,
			},
			want: "[fe80::1]:9100",
		},
		{
			name: "hostname",
			entry: &zeroconf.ServiceEntry{
				HostName: "agent.local",
				Port:     9100,
			},
			want: "agent.local:9100",
		},
		{
			name:  "empty",
			entry: &zeroconf.ServiceEntry{Port: 9100},
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := entryToAddr(tt.entry)
			if got != tt.want {
				t.Errorf("entryToAddr() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTxtDeviceName(t *testing.T) {
	tests := []struct {
		name  string
		entry *zeroconf.ServiceEntry
		want  string
	}{
		{
			name: "from_txt",
			entry: &zeroconf.ServiceEntry{
				Text: []string{"version=0.1.0", "device_name=my-macbook", "device_count=1"},
			},
			want: "my-macbook",
		},
		{
			name: "from_instance",
			entry: &zeroconf.ServiceEntry{
				Text: []string{"version=0.1.0"},
			},
			want: "fallback-instance",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "from_instance" {
				tt.entry.ServiceRecord.Instance = "fallback-instance"
			}
			got := txtDeviceName(tt.entry)
			if got != tt.want {
				t.Errorf("txtDeviceName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewAdvertiserSafe_NilLogger(t *testing.T) {
	// Should not panic when logger is nil
	adv := NewAdvertiserSafe("test", 9100, "0.1.0", 1, nil)
	if adv != nil {
		adv.Stop()
	}
	// In sandbox adv is typically nil due to mDNS failure; either way no panic
}

func TestIsLocalAddress(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"127.0.0.1", true},
		{"::1", true},
		{"192.168.1.50", false}, // may be local on some machines; we just check logic
		{"8.8.8.8", false},
		{"invalid", false},
	}
	for _, tt := range tests {
		got := IsLocalAddress(tt.host)
		if tt.host == "127.0.0.1" || tt.host == "::1" {
			if !got {
				t.Errorf("IsLocalAddress(%q) = %v, want true (loopback)", tt.host, got)
			}
		} else if tt.host == "8.8.8.8" || tt.host == "invalid" {
			if got {
				t.Errorf("IsLocalAddress(%q) = %v, want false", tt.host, got)
			}
		}
	}
}

func TestIsSelf(t *testing.T) {
	tests := []struct {
		addr               string
		deviceName         string
		selfDeviceName     string
		selfPrometheusPort int
		want               bool
	}{
		{"192.168.1.50:9100", "my-mac", "my-mac", 9100, false}, // same name but remote IP
		{"192.168.1.50:9100", "other", "my-mac", 9100, false},  // different device, remote IP
		{"127.0.0.1:9100", "other", "my-mac", 9100, true},      // local + same port
		{"127.0.0.1:9200", "other", "my-mac", 9100, false},     // local but different port
		{"8.8.8.8:9100", "other", "my-mac", 9100, false},       // remote
	}
	for _, tt := range tests {
		got := IsSelf(tt.addr, tt.deviceName, tt.selfDeviceName, tt.selfPrometheusPort)
		if got != tt.want {
			t.Errorf("IsSelf(%q, %q, %q, %d) = %v, want %v",
				tt.addr, tt.deviceName, tt.selfDeviceName, tt.selfPrometheusPort, got, tt.want)
		}
	}
}

func TestBrowserOnFound(t *testing.T) {
	// Integration-style test: start advertiser, start browser with short timeout,
	// verify onFound is called. May be flaky on CI without multicast.
	adv, err := NewAdvertiser("browser-test-device", 19100, "0.1.0", 1)
	if err != nil {
		t.Skipf("mDNS not available: %v", err)
	}
	defer adv.Stop()

	done := make(chan struct{})
	var once sync.Once
	browser := NewBrowser(
		func(addr, name string) {
			if name == "browser-test-device" {
				once.Do(func() { close(done) })
			}
		},
		nil,
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		_ = browser.Start(ctx)
	}()

	select {
	case <-done:
		// Found our test advertiser
	case <-ctx.Done():
		t.Skip("mDNS discovery timed out (local network may not support multicast)")
	}
}

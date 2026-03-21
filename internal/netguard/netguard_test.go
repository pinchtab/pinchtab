package netguard

import (
	"context"
	"errors"
	"net"
	"testing"
)

func stubResolveHostIPs(t *testing.T, fn func(context.Context, string, string) ([]net.IP, error)) {
	t.Helper()
	old := ResolveHostIPs
	ResolveHostIPs = fn
	t.Cleanup(func() {
		ResolveHostIPs = old
	})
}

func TestIsLocalHost(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{host: "localhost", want: true},
		{host: "LOCALHOST.", want: true},
		{host: "api.localhost", want: true},
		{host: "127.0.0.1", want: true},
		{host: "::1", want: true},
		{host: "::ffff:127.0.0.1", want: true},
		{host: "93.184.216.34", want: false},
		{host: "example.com", want: false},
	}

	for _, tt := range tests {
		if got := IsLocalHost(tt.host); got != tt.want {
			t.Fatalf("IsLocalHost(%q) = %v, want %v", tt.host, got, tt.want)
		}
	}
}

func TestValidatePublicIP(t *testing.T) {
	tests := []struct {
		name    string
		rawIP   string
		wantErr bool
	}{
		{name: "public ipv4", rawIP: "93.184.216.34", wantErr: false},
		{name: "public ipv6", rawIP: "2606:2800:220:1:248:1893:25c8:1946", wantErr: false},
		{name: "private ipv4", rawIP: "192.168.1.10", wantErr: true},
		{name: "loopback ipv4", rawIP: "127.0.0.1", wantErr: true},
		{name: "loopback mapped ipv6", rawIP: "::ffff:127.0.0.1", wantErr: true},
		{name: "shared address space", rawIP: "100.64.0.10", wantErr: true},
		{name: "benchmark network", rawIP: "198.18.0.10", wantErr: true},
		{name: "metadata link-local", rawIP: "169.254.169.254", wantErr: true},
	}

	for _, tt := range tests {
		err := ValidatePublicIP(net.ParseIP(tt.rawIP))
		if (err != nil) != tt.wantErr {
			t.Fatalf("ValidatePublicIP(%q) error = %v, wantErr %v", tt.rawIP, err, tt.wantErr)
		}
		if tt.wantErr && !errors.Is(err, ErrPrivateInternalIP) {
			t.Fatalf("ValidatePublicIP(%q) error = %v, want private/internal sentinel", tt.rawIP, err)
		}
	}
}

func TestResolveAndValidatePublicIPs(t *testing.T) {
	stubResolveHostIPs(t, func(ctx context.Context, network, host string) ([]net.IP, error) {
		switch host {
		case "public.example":
			return []net.IP{net.ParseIP("93.184.216.34"), net.ParseIP("2606:2800:220:1:248:1893:25c8:1946")}, nil
		case "mixed.example":
			return []net.IP{net.ParseIP("93.184.216.34"), net.ParseIP("10.0.0.8")}, nil
		case "dupe.example":
			return []net.IP{net.ParseIP("93.184.216.34"), net.ParseIP("93.184.216.34")}, nil
		default:
			return nil, errors.New("not found")
		}
	})

	ips, err := ResolveAndValidatePublicIPs(context.Background(), "public.example")
	if err != nil {
		t.Fatalf("ResolveAndValidatePublicIPs(public.example) error = %v", err)
	}
	if len(ips) != 2 {
		t.Fatalf("ResolveAndValidatePublicIPs(public.example) len = %d, want 2", len(ips))
	}

	ips, err = ResolveAndValidatePublicIPs(context.Background(), "dupe.example")
	if err != nil {
		t.Fatalf("ResolveAndValidatePublicIPs(dupe.example) error = %v", err)
	}
	if len(ips) != 1 {
		t.Fatalf("ResolveAndValidatePublicIPs(dupe.example) len = %d, want 1", len(ips))
	}

	if _, err := ResolveAndValidatePublicIPs(context.Background(), "mixed.example"); !errors.Is(err, ErrPrivateInternalIP) {
		t.Fatalf("ResolveAndValidatePublicIPs(mixed.example) error = %v, want private/internal sentinel", err)
	}

	if _, err := ResolveAndValidatePublicIPs(context.Background(), "missing.example"); !errors.Is(err, ErrResolveHost) {
		t.Fatalf("ResolveAndValidatePublicIPs(missing.example) error = %v, want resolve sentinel", err)
	}

	if _, err := ResolveAndValidatePublicIPs(context.Background(), "127.0.0.1"); !errors.Is(err, ErrPrivateInternalIP) {
		t.Fatalf("ResolveAndValidatePublicIPs(127.0.0.1) error = %v, want private/internal sentinel", err)
	}
}

func TestValidateRemoteIPAddress(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr error
	}{
		{name: "empty", raw: "", wantErr: nil},
		{name: "public ipv4", raw: "93.184.216.34", wantErr: nil},
		{name: "bracketed public ipv6", raw: "[2606:2800:220:1:248:1893:25c8:1946]", wantErr: nil},
		{name: "loopback", raw: "127.0.0.1", wantErr: ErrPrivateInternalIP},
		{name: "mapped loopback", raw: "::ffff:127.0.0.1", wantErr: ErrPrivateInternalIP},
		{name: "garbage", raw: "not-an-ip", wantErr: ErrUnparseableRemoteIP},
	}

	for _, tt := range tests {
		err := ValidateRemoteIPAddress(tt.raw)
		if tt.wantErr == nil {
			if err != nil {
				t.Fatalf("ValidateRemoteIPAddress(%q) error = %v, want nil", tt.raw, err)
			}
			continue
		}
		if !errors.Is(err, tt.wantErr) {
			t.Fatalf("ValidateRemoteIPAddress(%q) error = %v, want %v", tt.raw, err, tt.wantErr)
		}
	}
}

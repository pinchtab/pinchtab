package geo

import (
	"context"
	"strings"
	"testing"
)

func TestNoop_ReturnsZeroInfo(t *testing.T) {
	got, err := Noop{}.Lookup(context.Background(), "1.2.3.4")
	if err != nil {
		t.Fatalf("Noop.Lookup() error = %v", err)
	}
	if !got.IsZero() {
		t.Fatalf("Noop.Lookup() = %+v, want zero", got)
	}
}

func TestNoop_EmptyIPSafe(t *testing.T) {
	got, err := Noop{}.Lookup(context.Background(), "")
	if err != nil {
		t.Fatalf("Noop.Lookup(\"\") error = %v", err)
	}
	if !got.IsZero() {
		t.Fatalf("Noop.Lookup(\"\") = %+v, want zero", got)
	}
}

func TestStatic_ReturnsConfiguredInfo(t *testing.T) {
	want := Info{
		Timezone:   "Europe/London",
		Locale:     "en-GB",
		WebRTCIP:   "203.0.113.7",
		CountryISO: "GB",
	}
	got, err := Static{Info: want}.Lookup(context.Background(), "ignored")
	if err != nil {
		t.Fatalf("Static.Lookup() error = %v", err)
	}
	if got != want {
		t.Fatalf("Static.Lookup() = %+v, want %+v", got, want)
	}
}

func TestStatic_EmptyIPStillReturnsConfigured(t *testing.T) {
	want := Info{Timezone: "Europe/London"}
	got, _ := Static{Info: want}.Lookup(context.Background(), "")
	if got != want {
		t.Fatalf("Static.Lookup(\"\") = %+v, want %+v", got, want)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		info    Info
		wantErr bool
		wantSub string
	}{
		{name: "zero info accepted"},
		{name: "full good info", info: Info{
			Timezone: "Europe/London", Locale: "en-GB", WebRTCIP: "203.0.113.7", CountryISO: "GB",
		}},
		{name: "good language-only locale", info: Info{Locale: "en"}},
		{name: "good 3-letter language", info: Info{Locale: "fil"}},
		{name: "good ipv6 webrtc", info: Info{WebRTCIP: "2001:db8::1"}},
		{name: "bad timezone", info: Info{Timezone: "Not/AZone"}, wantErr: true, wantSub: "timezone"},
		{name: "bad locale lowercase region", info: Info{Locale: "en-gb"}, wantErr: true, wantSub: "locale"},
		{name: "bad locale long region", info: Info{Locale: "en-GBR"}, wantErr: true, wantSub: "locale"},
		{name: "bad locale digit", info: Info{Locale: "e1"}, wantErr: true, wantSub: "locale"},
		{name: "bad webrtc IP", info: Info{WebRTCIP: "not-an-ip"}, wantErr: true, wantSub: "webrtcIP"},
		{name: "bad country lowercase", info: Info{CountryISO: "gb"}, wantErr: true, wantSub: "countryISO"},
		{name: "bad country 3-letter", info: Info{CountryISO: "GBR"}, wantErr: true, wantSub: "countryISO"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.info)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Validate(%+v) = nil, want error", tt.info)
				}
				if tt.wantSub != "" && !strings.Contains(err.Error(), tt.wantSub) {
					t.Errorf("Validate(%+v) error = %v, want substring %q", tt.info, err, tt.wantSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate(%+v) unexpected error = %v", tt.info, err)
			}
		})
	}
}

var _ Provider = Noop{}
var _ Provider = Static{}

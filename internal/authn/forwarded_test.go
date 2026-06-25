package authn

import (
	"crypto/tls"
	"net/http"
	"testing"
)

func requestWith(host string, tlsOn bool, headers map[string]string) *http.Request {
	r := &http.Request{Host: host, Header: http.Header{}}
	if tlsOn {
		r.TLS = &tls.ConnectionState{}
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

func TestRequestScheme(t *testing.T) {
	tests := []struct {
		name       string
		req        *http.Request
		trustProxy bool
		want       string
	}{
		{"nil request", nil, true, "http"},
		{"plain http", requestWith("example.com", false, nil), false, "http"},
		{"local TLS", requestWith("example.com", true, nil), false, "https"},
		{
			"forwarded proto ignored when untrusted",
			requestWith("example.com", false, map[string]string{"X-Forwarded-Proto": "https"}),
			false, "http",
		},
		{
			"forwarded proto honored when trusted",
			requestWith("example.com", false, map[string]string{"X-Forwarded-Proto": "https"}),
			true, "https",
		},
		{
			"forwarded proto first value wins",
			requestWith("example.com", false, map[string]string{"X-Forwarded-Proto": "https, http"}),
			true, "https",
		},
		{
			"forwarded proto uppercase normalized",
			requestWith("example.com", false, map[string]string{"X-Forwarded-Proto": "HTTPS"}),
			true, "https",
		},
		{
			"rfc7239 Forwarded proto",
			requestWith("example.com", false, map[string]string{"Forwarded": `for=1.2.3.4;proto=https;host=example.com`}),
			true, "https",
		},
		{
			"rfc7239 Forwarded proto quoted",
			requestWith("example.com", false, map[string]string{"Forwarded": `proto="https"`}),
			true, "https",
		},
		{
			"X-Forwarded-Proto preferred over Forwarded",
			requestWith("example.com", false, map[string]string{"X-Forwarded-Proto": "https", "Forwarded": "proto=http"}),
			true, "https",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RequestScheme(tt.req, tt.trustProxy); got != tt.want {
				t.Fatalf("RequestScheme() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRequestIsHTTPS(t *testing.T) {
	r := requestWith("example.com", false, map[string]string{"X-Forwarded-Proto": "https"})
	if !RequestIsHTTPS(r, true) {
		t.Fatal("expected HTTPS when trusted X-Forwarded-Proto is https")
	}
	if RequestIsHTTPS(r, false) {
		t.Fatal("expected non-HTTPS when proxy headers are not trusted")
	}
}

func TestRequestHost(t *testing.T) {
	tests := []struct {
		name       string
		req        *http.Request
		trustProxy bool
		want       string
	}{
		{"nil request", nil, true, ""},
		{"direct host", requestWith("example.com", false, nil), false, "example.com"},
		{
			"forwarded host ignored when untrusted",
			requestWith("origin.internal", false, map[string]string{"X-Forwarded-Host": "public.example.com"}),
			false, "origin.internal",
		},
		{
			"forwarded host honored when trusted",
			requestWith("origin.internal", false, map[string]string{"X-Forwarded-Host": "public.example.com"}),
			true, "public.example.com",
		},
		{
			"forwarded host first value wins",
			requestWith("origin.internal", false, map[string]string{"X-Forwarded-Host": "public.example.com, proxy.internal"}),
			true, "public.example.com",
		},
		{
			"rfc7239 Forwarded host",
			requestWith("origin.internal", false, map[string]string{"Forwarded": `host=public.example.com;proto=https`}),
			true, "public.example.com",
		},
		{
			"falls back to direct host when no forwarded header",
			requestWith("origin.internal", false, nil),
			true, "origin.internal",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RequestHost(tt.req, tt.trustProxy); got != tt.want {
				t.Fatalf("RequestHost() = %q, want %q", got, tt.want)
			}
		})
	}
}

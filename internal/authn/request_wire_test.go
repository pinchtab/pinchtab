package authn

import (
	"bufio"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestCredentialsFromRequest_WireParsedCookieHeader(t *testing.T) {
	const want = "wire-cookie-token"
	raw := strings.Join([]string{
		"POST /action HTTP/1.1",
		"Host: localhost:9867",
		"Origin: http://localhost:9867",
		"Content-Type: application/json",
		"Cookie: " + CookieName + "=" + url.QueryEscape(want),
		"Content-Length: 2",
		"",
		"{}",
	}, "\r\n")

	req, err := http.ReadRequest(bufio.NewReader(strings.NewReader(raw)))
	if err != nil {
		t.Fatalf("ReadRequest() error = %v", err)
	}

	creds := CredentialsFromRequest(req)
	if creds.Value != want {
		t.Fatalf("CredentialsFromRequest().Value = %q, want %q", creds.Value, want)
	}
	if creds.Method != MethodCookie {
		t.Fatalf("CredentialsFromRequest().Method = %q, want %q", creds.Method, MethodCookie)
	}
}

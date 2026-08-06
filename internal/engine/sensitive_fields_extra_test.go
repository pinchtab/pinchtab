package engine

import "testing"

func TestIsSensitiveFieldAttrsMatrix(t *testing.T) {
	cases := []struct {
		name                                 string
		typ, autocomplete, nm, id, ariaLabel string
		want                                 bool
	}{
		{name: "password input", typ: "password", want: true},
		{name: "password uppercase", typ: "PASSWORD", want: true},
		{name: "current-password", typ: "text", autocomplete: "current-password", want: true},
		{name: "one-time-code", typ: "text", autocomplete: "one-time-code", want: true},
		{name: "cc-number", typ: "text", autocomplete: "cc-number", want: true},
		{name: "totp name hint", typ: "text", nm: "totp_code", want: true},
		{name: "apiKey id hint", typ: "text", id: "apiKey", want: true},
		{name: "aria-label secret", typ: "text", ariaLabel: "Secret token", want: true},
		{name: "revealed password", typ: "text", nm: "user_pwd", want: true},
		{name: "plain email", typ: "email", nm: "email", id: "login-email", want: false},
		{name: "search box", typ: "text", nm: "q", ariaLabel: "Search", want: false},
		{name: "empty", want: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsSensitiveFieldAttrs(c.typ, c.autocomplete, c.nm, c.id, c.ariaLabel); got != c.want {
				t.Fatalf("got %v want %v", got, c.want)
			}
		})
	}
}

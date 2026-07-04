package audit

import (
	"strings"
	"testing"
)

func TestParseCookieFlag(t *testing.T) {
	c, err := ParseCookieFlag("session=abc123")
	if err != nil {
		t.Fatalf("ParseCookieFlag: %v", err)
	}
	if c.Name != "session" || c.Value != "abc123" {
		t.Errorf("cookie = %+v", c)
	}

	if c, err := ParseCookieFlag("k=v=with=equals"); err != nil || c.Value != "v=with=equals" {
		t.Errorf("value with equals: %+v, %v", c, err)
	}

	for _, bad := range []string{"", "novalue", "=v", "name="} {
		if _, err := ParseCookieFlag(bad); err == nil {
			t.Errorf("ParseCookieFlag(%q) should error", bad)
		}
	}
}

func TestParseCookiesFile(t *testing.T) {
	data := []byte(`[
		{"name":"session","value":"abc","domain":"fixtures","path":"/","httpOnly":true},
		{"name":"pref","value":"dark"}
	]`)
	cookies, err := ParseCookiesFile(data)
	if err != nil {
		t.Fatalf("ParseCookiesFile: %v", err)
	}
	if len(cookies) != 2 || cookies[0].Domain != "fixtures" || !cookies[0].HTTPOnly {
		t.Errorf("cookies = %+v", cookies)
	}
}

func TestParseCookiesFileErrors(t *testing.T) {
	if _, err := ParseCookiesFile([]byte("{not json")); err == nil {
		t.Error("malformed JSON should error")
	} else if !strings.Contains(err.Error(), "expected a JSON array") {
		t.Errorf("malformed JSON error should explain the expected shape, got %v", err)
	}

	if _, err := ParseCookiesFile([]byte(`[{"name":"","value":"x"}]`)); err == nil {
		t.Error("missing name should error")
	}
	if _, err := ParseCookiesFile([]byte(`[{"name":"x","value":""}]`)); err == nil {
		t.Error("missing value should error")
	}
	if _, err := ParseCookiesFile([]byte(`{"name":"x","value":"y"}`)); err == nil {
		t.Error("non-array JSON should error")
	}
}

func TestCollectCookies(t *testing.T) {
	cookies, err := CollectCookies([]string{"a=1", "b=2"}, []byte(`[{"name":"c","value":"3"}]`))
	if err != nil {
		t.Fatalf("CollectCookies: %v", err)
	}
	if len(cookies) != 3 || cookies[0].Name != "a" || cookies[2].Name != "c" {
		t.Errorf("cookies = %+v", cookies)
	}

	if _, err := CollectCookies([]string{"bad"}, nil); err == nil {
		t.Error("bad flag should error")
	}
	if _, err := CollectCookies(nil, []byte("bad")); err == nil {
		t.Error("bad file should error")
	}
	if cookies, err := CollectCookies(nil, nil); err != nil || len(cookies) != 0 {
		t.Errorf("empty input = %+v, %v", cookies, err)
	}
}

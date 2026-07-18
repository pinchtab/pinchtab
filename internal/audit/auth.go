package audit

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Cookie is one cookie to inject before an audit run. The JSON shape
// matches the POST /cookies request entries (bridge SetCookieParams).
type Cookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain,omitempty"`
	Path     string  `json:"path,omitempty"`
	Secure   bool    `json:"secure,omitempty"`
	HTTPOnly bool    `json:"httpOnly,omitempty"`
	SameSite string  `json:"sameSite,omitempty"`
	Expires  float64 `json:"expires,omitempty"`
}

// ParseCookieFlag parses one --cookie "name=value" flag.
func ParseCookieFlag(s string) (Cookie, error) {
	name, value, ok := strings.Cut(s, "=")
	name = strings.TrimSpace(name)
	if !ok || name == "" || value == "" {
		return Cookie{}, fmt.Errorf("invalid --cookie %q: expected name=value", s)
	}
	return Cookie{Name: name, Value: value}, nil
}

// ParseCookiesFile parses a cookies JSON file: an array of cookie objects
// in the SetCookieParams shape. Every entry must carry name and value.
func ParseCookiesFile(data []byte) ([]Cookie, error) {
	var cookies []Cookie
	if err := json.Unmarshal(data, &cookies); err != nil {
		return nil, fmt.Errorf("parse cookies file: %w (expected a JSON array of {name, value, ...} objects)", err)
	}
	for i, c := range cookies {
		if strings.TrimSpace(c.Name) == "" || c.Value == "" {
			return nil, fmt.Errorf("cookies file entry %d: name and value are required", i)
		}
	}
	return cookies, nil
}

// CollectCookies merges repeatable --cookie flags with an optional cookies
// file (flags first, then file entries).
func CollectCookies(flags []string, fileData []byte) ([]Cookie, error) {
	var cookies []Cookie
	for _, f := range flags {
		c, err := ParseCookieFlag(f)
		if err != nil {
			return nil, err
		}
		cookies = append(cookies, c)
	}
	if len(fileData) > 0 {
		fromFile, err := ParseCookiesFile(fileData)
		if err != nil {
			return nil, err
		}
		cookies = append(cookies, fromFile...)
	}
	return cookies, nil
}

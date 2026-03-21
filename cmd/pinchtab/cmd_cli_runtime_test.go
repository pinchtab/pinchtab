package main

import (
	"os"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestResolveCLIBase(t *testing.T) {
	tests := []struct {
		name       string
		serverFlag string
		envURL     string
		cfgBind    string
		cfgPort    string
		expected   string
	}{
		{
			name:       "--server overrides everything",
			serverFlag: "http://remote:1234",
			envURL:     "http://env:5678",
			cfgBind:    "localhost",
			cfgPort:    "9000",
			expected:   "http://remote:1234",
		},
		{
			name:       "--server trims trailing slash",
			serverFlag: "http://remote:1234/",
			expected:   "http://remote:1234",
		},
		{
			name:       "PINCHTAB_URL overrides config",
			envURL:     "http://env:5678",
			cfgBind:    "localhost",
			cfgPort:    "9000",
			expected:   "http://env:5678",
		},
		{
			name:       "PINCHTAB_URL trims trailing slash",
			envURL:     "http://env:5678/",
			expected:   "http://env:5678",
		},
		{
			name:     "default fallback uses 127.0.0.1 and 9867",
			expected: "http://127.0.0.1:9867",
		},
		{
			name:     "custom port fallback",
			cfgPort:  "8080",
			expected: "http://127.0.0.1:8080",
		},
		{
			name:     "server.bind=127.0.0.1 uses that host",
			cfgBind:  "127.0.0.1",
			cfgPort:  "8080",
			expected: "http://127.0.0.1:8080",
		},
		{
			name:     "server.bind=localhost uses that host",
			cfgBind:  "localhost",
			cfgPort:  "8080",
			expected: "http://localhost:8080",
		},
		{
			name:     "server.bind=::1 produces bracketed IPv6 URL",
			cfgBind:  "::1",
			cfgPort:  "8080",
			expected: "http://[::1]:8080",
		},
		{
			name:     "server.bind=[::1] handles pre-bracketed IPv6 URL",
			cfgBind:  "[::1]",
			cfgPort:  "8080",
			expected: "http://[::1]:8080",
		},
		{
			name:     "server.bind=0.0.0.0 falls back to loopback",
			cfgBind:  "0.0.0.0",
			cfgPort:  "8080",
			expected: "http://127.0.0.1:8080",
		},
		{
			name:     "server.bind=:: falls back to loopback",
			cfgBind:  "::",
			cfgPort:  "8080",
			expected: "http://127.0.0.1:8080",
		},
		{
			name:     "non-loopback bind falls back to loopback",
			cfgBind:  "192.168.1.10",
			cfgPort:  "8080",
			expected: "http://127.0.0.1:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore global state
			oldServerURL := serverURL
			serverURL = tt.serverFlag
			defer func() { serverURL = oldServerURL }()

			if tt.envURL != "" {
				os.Setenv("PINCHTAB_URL", tt.envURL)
				defer os.Unsetenv("PINCHTAB_URL")
			} else {
				os.Unsetenv("PINCHTAB_URL")
			}

			cfg := &config.RuntimeConfig{}
			cfg.Bind = tt.cfgBind
			cfg.Port = tt.cfgPort

			actual := resolveCLIBase(cfg)
			if actual != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, actual)
			}
		})
	}
}

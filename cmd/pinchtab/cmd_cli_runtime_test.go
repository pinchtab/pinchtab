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
		expected   string
	}{
		{
			name:       "--server overrides everything",
			serverFlag: "http://remote:1234",
			envURL:     "http://env:5678",
			expected:   "http://remote:1234",
		},
		{
			name:       "--server trims trailing slash",
			serverFlag: "http://remote:1234/",
			expected:   "http://remote:1234",
		},
		{
			name:       "PINCHTAB_SERVER overrides fallback",
			envURL:     "http://env:5678",
			expected:   "http://env:5678",
		},
		{
			name:       "PINCHTAB_SERVER trims trailing slash",
			envURL:     "http://env:5678/",
			expected:   "http://env:5678",
		},
		{
			name:     "default fallback uses 127.0.0.1 and 9867",
			expected: "http://127.0.0.1:9867",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore global state
			oldServerURL := serverURL
			serverURL = tt.serverFlag
			defer func() { serverURL = oldServerURL }()

			if tt.envURL != "" {
				os.Setenv("PINCHTAB_SERVER", tt.envURL)
				defer os.Unsetenv("PINCHTAB_SERVER")
			} else {
				os.Unsetenv("PINCHTAB_SERVER")
			}

			cfg := &config.RuntimeConfig{}

			actual := resolveCLIBase(cfg)
			if actual != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, actual)
			}
		})
	}
}

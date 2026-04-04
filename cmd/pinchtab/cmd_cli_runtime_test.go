package main

import (
	"io"
	"net/http"
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
			name:     "PINCHTAB_SERVER overrides fallback",
			envURL:   "http://env:5678",
			expected: "http://env:5678",
		},
		{
			name:     "PINCHTAB_SERVER trims trailing slash",
			envURL:   "http://env:5678/",
			expected: "http://env:5678",
		},
		{
			name:     "default fallback uses 127.0.0.1 and instancePortStart",
			expected: "http://127.0.0.1:9868",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore global state
			oldServerURL := serverURL
			serverURL = tt.serverFlag
			defer func() { serverURL = oldServerURL }()

			if tt.envURL != "" {
				t.Setenv("PINCHTAB_SERVER", tt.envURL)
			} else {
				t.Setenv("PINCHTAB_SERVER", "")
			}

			cfg := &config.RuntimeConfig{}

			actual := resolveCLIBase(cfg)
			if actual != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, actual)
			}
		})
	}
}

func TestResolveCLIAgentID(t *testing.T) {
	tests := []struct {
		name      string
		flagValue string
		envValue  string
		expected  string
	}{
		{
			name:      "--agent-id overrides environment",
			flagValue: "agent-flag",
			envValue:  "agent-env",
			expected:  "agent-flag",
		},
		{
			name:      "--agent-id trims whitespace",
			flagValue: "  agent-flag  ",
			expected:  "agent-flag",
		},
		{
			name:      "blank --agent-id falls through to environment",
			flagValue: "   ",
			envValue:  "agent-env",
			expected:  "agent-env",
		},
		{
			name:     "PINCHTAB_AGENT_ID overrides default",
			envValue: "agent-env",
			expected: "agent-env",
		},
		{
			name:     "PINCHTAB_AGENT_ID trims whitespace",
			envValue: "  agent-env  ",
			expected: "agent-env",
		},
		{
			name:      "blank values fall back to cli",
			flagValue: "   ",
			envValue:  "   ",
			expected:  "cli",
		},
		{
			name:     "default fallback is cli",
			expected: "cli",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldAgentID := cliAgentID
			cliAgentID = tt.flagValue
			defer func() { cliAgentID = oldAgentID }()

			t.Setenv("PINCHTAB_AGENT_ID", tt.envValue)

			if got := resolveCLIAgentID(); got != tt.expected {
				t.Fatalf("resolveCLIAgentID() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestRunCLIWithInjectsAgentIDHeaders(t *testing.T) {
	const wantAgentID = "agent-main"

	var gotRequest *http.Request
	client := &http.Client{
		Transport: agentHeaderTransport{
			base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				gotRequest = req
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(http.NoBody),
					Request:    req,
				}, nil
			}),
			agentID: wantAgentID,
		},
	}

	req, err := http.NewRequest(http.MethodGet, "http://example.test/health", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	if _, err := client.Do(req); err != nil {
		t.Fatalf("client.Do() error = %v", err)
	}

	if gotRequest == nil {
		t.Fatal("transport did not receive request")
	}
	if got := gotRequest.Header.Get("X-Agent-Id"); got != wantAgentID {
		t.Fatalf("X-Agent-Id = %q, want %q", got, wantAgentID)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

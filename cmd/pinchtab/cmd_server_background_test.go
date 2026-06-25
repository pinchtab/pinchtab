package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/server"
)

func TestIsBackgroundServerReadyRequiresValidPinchTabHealth(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{
			name:   "pinchtab health",
			status: http.StatusOK,
			body:   `{"status":"ok","mode":"dashboard","version":"dev","marker":"marker-123"}`,
			want:   true,
		},
		{
			name:   "unauthorized pinchtab health",
			status: http.StatusUnauthorized,
			body:   `{"code":"bad_token","message":"unauthorized"}`,
			want:   false,
		},
		{
			name:   "plain ok from other service",
			status: http.StatusOK,
			body:   `ok`,
			want:   false,
		},
		{
			name:   "partial json from other service",
			status: http.StatusOK,
			body:   `{"status":"ok"}`,
			want:   false,
		},
		{
			name:   "valid health with wrong marker",
			status: http.StatusOK,
			body:   `{"status":"ok","mode":"dashboard","version":"dev","marker":"other"}`,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/health/background" {
					t.Fatalf("path = %q, want /health/background", r.URL.Path)
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			if got := isBackgroundServerReady(srv.URL, "marker-123"); got != tt.want {
				t.Fatalf("isBackgroundServerReady() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsBackgroundServerReadyDoesNotSendBearerToken(t *testing.T) {
	var gotAuth string
	var gotMarker string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotMarker = r.Header.Get(backgroundHealthProbeHeader)
		_, _ = w.Write([]byte(`{"status":"ok","mode":"dashboard","version":"dev","marker":"marker-123"}`))
	}))
	defer srv.Close()

	if !isBackgroundServerReady(srv.URL, "marker-123") {
		t.Fatal("expected background health probe to succeed")
	}
	if gotAuth != "" {
		t.Fatalf("Authorization = %q, want empty", gotAuth)
	}
	if gotMarker != "marker-123" {
		t.Fatalf("%s = %q, want marker", backgroundHealthProbeHeader, gotMarker)
	}
}

func TestBackgroundServerArgsForwardsForegroundFlags(t *testing.T) {
	got := backgroundServerArgs("marker-123", serverBackgroundOptions{
		Yolo:       true,
		Headed:     true,
		Verbose:    true,
		Extensions: []string{"./ext-one", "/tmp/ext two"},
	})
	want := []string{
		"server", "--background-child", "marker-123",
		"-y",
		"-H",
		"-v",
		"-e", "./ext-one",
		"-e", "/tmp/ext two",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("backgroundServerArgs() = %#v, want %#v", got, want)
	}
}

func TestServerPIDCommandMatchesRequiresExpectedMetadata(t *testing.T) {
	info := serverPIDInfo{
		Executable: "/tmp/pinchtab",
		Marker:     "marker-123",
	}

	tests := []struct {
		name    string
		command string
		want    bool
	}{
		{name: "match", command: "/tmp/pinchtab server --background-child marker-123 -y", want: true},
		{name: "wrong marker", command: "/tmp/pinchtab server --background-child other", want: false},
		{name: "wrong subcommand", command: "/tmp/pinchtab bridge --background-child marker-123", want: false},
		{name: "wrong executable", command: "/tmp/other server --background-child marker-123", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := serverPIDCommandMatches(tt.command, info); got != tt.want {
				t.Fatalf("serverPIDCommandMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVerifyServerPIDInfoRefusesLegacyPID(t *testing.T) {
	err := verifyServerPIDInfo(serverPIDInfo{PID: os.Getpid()})
	if err == nil || !strings.Contains(err.Error(), "lacks verifiable background metadata") {
		t.Fatalf("verifyServerPIDInfo() error = %v, want missing metadata error", err)
	}
}

func TestStopViaAPIShutdownEndpoint(t *testing.T) {
	shutdownCalled := false
	healthAlive := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			if healthAlive {
				w.WriteHeader(200)
				_, _ = w.Write([]byte(`{"status":"ok","mode":"dashboard","version":"dev"}`))
			} else {
				w.WriteHeader(503)
			}
		case "/shutdown":
			if r.Method != http.MethodPost {
				w.WriteHeader(405)
				return
			}
			shutdownCalled = true
			healthAlive = false
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{"status":"shutting down"}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	port := strings.TrimPrefix(srv.URL, "http://127.0.0.1:")

	err := server.ShutdownServer(port, "")
	if err != nil {
		t.Fatalf("ShutdownServer() error = %v", err)
	}
	if !shutdownCalled {
		t.Fatal("POST /shutdown was not called")
	}
}

func TestVerifyServerPIDInfoChecksProcessCommand(t *testing.T) {
	orig := readProcessCommand
	readProcessCommand = func(pid int) (string, error) {
		return "/tmp/pinchtab server --background-child marker-123", nil
	}
	defer func() {
		readProcessCommand = orig
	}()

	err := verifyServerPIDInfo(serverPIDInfo{
		PID:        os.Getpid(),
		Executable: "/tmp/pinchtab",
		Marker:     "marker-123",
	})
	if err != nil {
		t.Fatalf("verifyServerPIDInfo() error = %v", err)
	}
}

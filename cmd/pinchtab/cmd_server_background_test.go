package main

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/daemon"
	"github.com/pinchtab/pinchtab/internal/server"
	"github.com/spf13/pflag"
)

func TestDetachedDaemonOwnershipTreatsUnsupportedOSAsNotInstalled(t *testing.T) {
	original := daemonInstallationStatus
	defer func() { daemonInstallationStatus = original }()

	daemonInstallationStatus = func() (bool, error) {
		return false, fmt.Errorf("resolve manager: %w", daemon.ErrUnsupportedOS)
	}
	installed, err := detachedDaemonOwnership()
	if err != nil {
		t.Fatalf("unsupported OS must not be an ownership error, got %v", err)
	}
	if installed {
		t.Fatal("a daemon cannot own the server on an OS that cannot host one")
	}

	daemonInstallationStatus = func() (bool, error) {
		return false, errors.New("service path unreadable")
	}
	if _, err := detachedDaemonOwnership(); err == nil {
		t.Fatal("a genuine ownership error must still propagate")
	}
}

func TestApplyServerAddressFlagsReportsAddressChange(t *testing.T) {
	cases := []struct {
		name       string
		bind, port string
		flagBind   string
		flagPort   string
		want       bool
	}{
		{"no flags", "127.0.0.1", "9867", "", "", false},
		{"port equals default", "127.0.0.1", "9867", "", "9867", false},
		{"bind equals default", "127.0.0.1", "9867", "127.0.0.1", "", false},
		{"blank flags", "127.0.0.1", "9867", "  ", "  ", false},
		{"port differs", "127.0.0.1", "9867", "", "5000", true},
		{"bind differs", "127.0.0.1", "9867", "0.0.0.0", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.RuntimeConfig{Bind: tc.bind, Port: tc.port}
			if got := applyServerAddressFlags(cfg, tc.flagBind, tc.flagPort); got != tc.want {
				t.Fatalf("applyServerAddressFlags() changed = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRequireDetachedServerOwnershipAllowsChangedAddress(t *testing.T) {
	original := daemonInstallationStatus
	defer func() { daemonInstallationStatus = original }()
	daemonInstallationStatus = func() (bool, error) { return true, nil }

	if err := requireDetachedServerOwnership("background start", true); err != nil {
		t.Fatalf("a genuinely different address must bypass the ownership gate, got %v", err)
	}
	err := requireDetachedServerOwnership("background start", false)
	if err == nil || !strings.Contains(err.Error(), "pinchtab daemon start") {
		t.Fatalf("the daemon-owned address must still defer to the installed daemon, got %v", err)
	}
}

func TestRequireDetachedServerOwnershipAllowsUnsupportedOS(t *testing.T) {
	original := daemonInstallationStatus
	defer func() { daemonInstallationStatus = original }()
	daemonInstallationStatus = func() (bool, error) {
		return false, fmt.Errorf("resolve manager: %w", daemon.ErrUnsupportedOS)
	}

	if err := requireDetachedServerOwnership("automatic start", false); err != nil {
		t.Fatalf("auto-start on an OS without daemon support must be allowed, got %v", err)
	}
}

func TestDetachedServerStartsRefuseInstalledOrUnknownServiceOwnership(t *testing.T) {
	original := daemonInstallationStatus
	defer func() { daemonInstallationStatus = original }()

	tests := []struct {
		name      string
		installed bool
		err       error
		start     func() error
		want      string
	}{
		{
			name: "background installed", installed: true,
			start: func() error { return runServerBackground(&config.RuntimeConfig{}, serverBackgroundOptions{}, false) },
			want:  "pinchtab daemon start",
		},
		{
			name: "automatic installed", installed: true,
			start: autoStartServer,
			want:  "pinchtab daemon start",
		},
		{
			name: "automatic unknown", err: errors.New("service path unreadable"),
			start: autoStartServer,
			want:  "refusing automatic start",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			daemonInstallationStatus = func() (bool, error) { return tt.installed, tt.err }
			err := tt.start()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("detached start error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestServerRestartRefusesInstalledOrUnknownServiceOwnership(t *testing.T) {
	original := daemonInstallationStatus
	defer func() { daemonInstallationStatus = original }()

	daemonInstallationStatus = func() (bool, error) { return true, nil }
	err := runServerRestart(&config.RuntimeConfig{})
	if err == nil || !strings.Contains(err.Error(), "pinchtab daemon restart") {
		t.Fatalf("installed-service error = %v", err)
	}

	daemonInstallationStatus = func() (bool, error) {
		return false, errors.New("service path unreadable")
	}
	err = runServerRestart(&config.RuntimeConfig{})
	if err == nil || !strings.Contains(err.Error(), "refusing restart") {
		t.Fatalf("unknown-ownership error = %v", err)
	}
}

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

func TestIsPinchTabAuthError(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{"pinchtab 401", 401, `{"code":"missing_token","error":"unauthorized"}`, true},
		{"pinchtab 403", 403, `{"code":"invalid_token","error":"unauthorized"}`, true},
		{"foreign 401 html", 401, `<html>401 Authorization Required</html>`, false},
		{"json missing code", 401, `{"error":"unauthorized"}`, false},
		{"ok response", 200, `{"code":"x","error":"y"}`, false},
		{"foreign 404", 404, `404 page not found`, false},
	}
	for _, tc := range cases {
		if got := isPinchTabAuthError(tc.status, []byte(tc.body)); got != tc.want {
			t.Errorf("%s: isPinchTabAuthError = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestPortBusyErrorAuthenticatedPinchTab(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"missing_token","error":"unauthorized"}`))
	}))
	defer srv.Close()

	err := portBusyError(srv.URL, "/home/user/.config/pinchtab/config.json")
	if err == nil {
		t.Fatal("expected an error for a busy port")
	}
	msg := err.Error()
	for _, want := range []string{
		"PinchTab server (different config/token)",
		srv.URL,
		"pinchtab server stop",
		`"port"`,
		"/home/user/.config/pinchtab/config.json",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "not a PinchTab server") {
		t.Errorf("authenticated PinchTab server mislabeled as foreign:\n%s", msg)
	}
}

func TestPortBusyErrorForeignListener(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>welcome to nginx</html>"))
	}))
	defer srv.Close()

	err := portBusyError(srv.URL, "/tmp/config.json")
	if err == nil {
		t.Fatal("expected an error for a busy port")
	}
	msg := err.Error()
	for _, want := range []string{"not a PinchTab server", srv.URL, `"port"`, "/tmp/config.json"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
}

func TestPortBusyErrorReadyPinchTab(t *testing.T) {
	for _, status := range []string{"ok", "degraded"} {
		t.Run(status, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"status":"` + status + `","mode":"dashboard","version":"dev"}`))
			}))
			defer srv.Close()

			err := portBusyError(srv.URL, "/tmp/config.json")
			if err == nil {
				t.Fatal("expected an error for a busy port")
			}
			if !strings.Contains(err.Error(), "server already running") || !strings.Contains(err.Error(), "pinchtab server stop") {
				t.Errorf("ready-server message lacks the stop command:\n%s", err)
			}
		})
	}
}

func TestPortBusyErrorFreePort(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	if err := portBusyError(url, "/tmp/config.json"); err != nil {
		t.Errorf("free port should yield nil, got %v", err)
	}
}

// The mode built to be diagnosed from a file must not need -v to record anything:
// the child inherits the default level, and only an explicit level travels.
func TestBackgroundServerArgsLogLevelForwarding(t *testing.T) {
	plain := backgroundServerArgs("marker-123", serverBackgroundOptions{})
	if slices.Contains(plain, "-v") {
		t.Errorf("a default background run should not force verbose: %#v", plain)
	}
	if slices.Contains(plain, "--log-level") {
		t.Errorf("a default background run should not pin a level: %#v", plain)
	}
	if !reflect.DeepEqual(plain, []string{"server", "--background-child", "marker-123"}) {
		t.Errorf("backgroundServerArgs() = %#v, want just the child marker", plain)
	}

	explicit := backgroundServerArgs("marker-123", serverBackgroundOptions{LogLevel: "warn"})
	want := []string{"server", "--background-child", "marker-123", "--log-level", "warn"}
	if !reflect.DeepEqual(explicit, want) {
		t.Errorf("backgroundServerArgs() = %#v, want %#v", explicit, want)
	}
}

func TestEveryFlagTheServerDeclaresTravelsToTheDetachedChild(t *testing.T) {
	notForwarded := map[string]string{
		"background":            "the flag that spawns the child; forwarding it would fork forever",
		backgroundChildFlagName: "the child marker, passed positionally by backgroundServerArgs itself",
	}
	freshServerFlags := func() *pflag.FlagSet {
		fs := pflag.NewFlagSet("server", pflag.ContinueOnError)
		serverCmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
			switch f.Value.Type() {
			case "bool":
				fs.BoolP(f.Name, f.Shorthand, false, "")
			case "string":
				fs.StringP(f.Name, f.Shorthand, "", "")
			case "stringArray":
				fs.StringArrayP(f.Name, f.Shorthand, nil, "")
			default:
				t.Fatalf("--%s has flag type %q the census cannot drive", f.Name, f.Value.Type())
			}
		})
		return fs
	}

	parent := freshServerFlags()
	var argv []string
	parent.VisitAll(func(f *pflag.Flag) {
		if _, exempt := notForwarded[f.Name]; exempt {
			return
		}
		if f.Value.Type() == "bool" {
			argv = append(argv, "--"+f.Name)
			return
		}
		argv = append(argv, "--"+f.Name, "value-of-"+f.Name)
		if f.Value.Type() == "stringArray" {
			argv = append(argv, "--"+f.Name, "second-value-of-"+f.Name)
		}
	})
	if err := parent.Parse(argv); err != nil {
		t.Fatalf("parse parent argv %v: %v", argv, err)
	}

	childArgv := backgroundServerArgs("marker", serverBackgroundOptionsFromFlags(parent))
	child := freshServerFlags()
	if err := child.Parse(childArgv[1:]); err != nil {
		t.Fatalf("parse child argv %v: %v", childArgv, err)
	}

	checked := 0
	parent.VisitAll(func(f *pflag.Flag) {
		if _, exempt := notForwarded[f.Name]; exempt {
			return
		}
		checked++
		got := child.Lookup(f.Name)
		if !got.Changed || got.Value.String() != f.Value.String() {
			t.Errorf("--%s=%s is declared by the server subcommand and applied by the parent, but the detached child parses it as %q (set=%v)", f.Name, f.Value, got.Value, got.Changed)
		}
	})
	if checked < 6 {
		t.Fatalf("checked only %d forwarded flags; this census would prove little", checked)
	}
	if got := child.Lookup(backgroundChildFlagName).Value.String(); got != "marker" {
		t.Errorf("child marker = %q, want %q", got, "marker")
	}
	if child.Changed("background") {
		t.Errorf("the detached child was handed --background and would fork again: %v", childArgv)
	}
	for _, inherited := range []string{"server", "agent-id"} {
		if serverCmd.LocalFlags().Lookup(inherited) != nil {
			t.Errorf("--%s is a root persistent flag yet reads as one the server declares; the census would then demand a client-side flag be forwarded to the server itself", inherited)
		}
	}
}

func TestAutoStartKeepsLiveVerifiedPIDFile(t *testing.T) {
	orig := readProcessCommand
	readProcessCommand = func(pid int) (string, error) {
		return "/tmp/pinchtab server --background-child marker-123", nil
	}
	defer func() { readProcessCommand = orig }()

	stateDir := t.TempDir()
	if err := writeServerPID(stateDir, serverPIDInfo{
		PID:        os.Getpid(),
		Executable: "/tmp/pinchtab",
		Args:       []string{"server", backgroundChildFlag, "marker-123"},
		Marker:     "marker-123",
	}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(serverPIDFilePath(stateDir))
	if err != nil {
		t.Fatal(err)
	}

	err = autoStartServerIn(stateDir)
	if err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("autoStartServerIn() error = %v, want already running", err)
	}
	after, err := os.ReadFile(serverPIDFilePath(stateDir))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("pid file overwritten:\nbefore %s\nafter  %s", before, after)
	}
}

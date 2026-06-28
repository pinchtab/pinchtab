package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIsServerHealthy_ReturnsTrue(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	if !isServerHealthy(ts.URL, "") {
		t.Fatal("expected healthy server to return true")
	}
}

func TestIsServerHealthy_ReturnsFalseForError(t *testing.T) {
	if isServerHealthy("http://127.0.0.1:1", "") {
		t.Fatal("expected unreachable server to return false")
	}
}

func TestIsServerHealthy_ReturnsFalseFor500(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	if isServerHealthy(ts.URL, "") {
		t.Fatal("expected 500 to return false")
	}
}

func TestIsServerHealthy_SendsAuthHeader(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	if !isServerHealthy(ts.URL, "test-token") {
		t.Fatal("expected healthy server with correct token to return true")
	}
	// 401 still means the server is running — isServerHealthy checks
	// reachability, not auth correctness.
	if !isServerHealthy(ts.URL, "wrong-token") {
		t.Fatal("expected running server with wrong token to still return true (401 < 500)")
	}
}

func TestIsServerHealthy_SendsSessionAuthHeader(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Session ses_testtoken" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	if !isServerHealthy(ts.URL, "ses_testtoken") {
		t.Fatal("expected healthy server with session token to return true")
	}
}

func TestWaitForServer_ImmediatelyHealthy(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	if !waitForServer(ts.URL, "", 5*time.Second) {
		t.Fatal("expected immediately healthy server to return true")
	}
}

func TestEnsureServerNoopWhenHealthy(t *testing.T) {
	started := false

	err := ensureServerWith(
		"http://127.0.0.1:9867",
		"",
		"test",
		func() error {
			started = true
			return nil
		},
		func(baseURL, token string) bool {
			return true
		},
		time.Second,
	)

	if err != nil {
		t.Fatalf("ensureServerWith() error = %v", err)
	}
	if started {
		t.Fatal("expected healthy server to skip auto-start")
	}
}

func TestEnsureServerStartsAndWaits(t *testing.T) {
	started := false
	healthChecks := 0

	err := ensureServerWith(
		"http://127.0.0.1:9867",
		"",
		"test",
		func() error {
			started = true
			return nil
		},
		func(baseURL, token string) bool {
			healthChecks++
			return started && healthChecks >= 2
		},
		time.Second,
	)

	if err != nil {
		t.Fatalf("ensureServerWith() error = %v", err)
	}
	if !started {
		t.Fatal("expected auto-start to run")
	}
	if healthChecks < 2 {
		t.Fatalf("expected health to be checked before and after start, got %d checks", healthChecks)
	}
}

func TestEnsureServerDoesNotStartWhenAutoStartDisabled(t *testing.T) {
	started := false

	err := ensureServerWithAutoStart(
		"http://127.0.0.1:9999",
		"",
		"test",
		false,
		func() error {
			started = true
			return nil
		},
		func(baseURL, token string) bool {
			return false
		},
		time.Second,
	)

	if err == nil || !strings.Contains(err.Error(), "auto-start is only supported") {
		t.Fatalf("ensureServerWithAutoStart() error = %v, want auto-start disabled error", err)
	}
	if started {
		t.Fatal("expected auto-start function not to run")
	}
}

func TestAutoStartServerArgsIgnoreClientServerURL(t *testing.T) {
	oldServerURL := serverURL
	serverURL = "http://127.0.0.1:9999"
	defer func() { serverURL = oldServerURL }()

	args := autoStartServerArgs()
	if len(args) != 1 || args[0] != "server" {
		t.Fatalf("autoStartServerArgs() = %v, want [server]", args)
	}
}

func TestEnsureServerReturnsStartError(t *testing.T) {
	wantErr := errors.New("boom")

	err := ensureServerWith(
		"http://127.0.0.1:9867",
		"",
		"test",
		func() error {
			return wantErr
		},
		func(baseURL, token string) bool {
			return false
		},
		time.Second,
	)

	if !errors.Is(err, wantErr) {
		t.Fatalf("ensureServerWith() error = %v, want wrapped %v", err, wantErr)
	}
}

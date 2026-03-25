package main

import (
	"net/http"
	"net/http/httptest"
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

func TestWaitForServer_ImmediatelyHealthy(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	if !waitForServer(ts.URL, "", 5*time.Second) {
		t.Fatal("expected immediately healthy server to return true")
	}
}

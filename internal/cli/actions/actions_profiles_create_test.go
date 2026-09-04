package actions

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestProfilesCreatePostsTheNameAndPrintsTheCreatedIdentity(t *testing.T) {
	var sent struct {
		Name string `json:"name"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/profiles" {
			t.Fatalf("request = %s %s, want POST /profiles", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = w.Write([]byte(`{"status":"created","id":"prof_1234","name":"work"}`))
	}))
	defer srv.Close()

	out := captureStdout(t, func() {
		ProfilesCreate(srv.Client(), srv.URL, "", "work")
	})

	if sent.Name != "work" {
		t.Fatalf("created name = %q, want work", sent.Name)
	}
	for _, want := range []string{"prof_1234", "work"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output %q does not contain %q", out, want)
		}
	}
}

func TestInstanceStartMissingProfilePrintsTheCreateRemedy(t *testing.T) {
	if os.Getenv("PINCHTAB_TEST_INSTANCE_START_MISSING") == "1" {
		cmd := &cobra.Command{}
		cmd.Flags().String("profile", "", "")
		cmd.Flags().String("mode", "", "")
		cmd.Flags().String("port", "", "")
		cmd.Flags().StringArray("extension", nil, "")
		cmd.Flags().StringArray("allow-domain", nil, "")
		cmd.Flags().String("browser", "", "")
		cmd.Flags().StringArray("browser-fallback", nil, "")
		_ = cmd.Flags().Set("profile", "missing")
		InstanceStart(http.DefaultClient, os.Getenv("PINCHTAB_TEST_SERVER"), "", cmd)
		return
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"profile_not_found","error":"profile missing","details":{"remedy":"pinchtab profiles create missing"}}`))
	}))
	defer srv.Close()

	cmd := exec.Command(os.Args[0], "-test.run=^TestInstanceStartMissingProfilePrintsTheCreateRemedy$")
	cmd.Env = append(os.Environ(), "PINCHTAB_TEST_INSTANCE_START_MISSING=1", "PINCHTAB_TEST_SERVER="+srv.URL)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("instance start unexpectedly succeeded for a missing profile")
	}
	if !strings.Contains(string(out), "Remedy: pinchtab profiles create missing") {
		t.Fatalf("CLI output does not carry the profile creation remedy:\n%s", out)
	}
}

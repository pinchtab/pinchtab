package actions

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

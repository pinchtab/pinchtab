package actions

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveTabRef(t *testing.T) {
	tabs := []struct {
		ID string `json:"id"`
	}{
		{ID: "AAAA1111"},
		{ID: "BBBB2222"},
		{ID: "CCCC3333"},
	}
	resp, _ := json.Marshal(map[string]any{"tabs": tabs})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(resp)
	}))
	defer srv.Close()

	client := srv.Client()

	t.Run("numeric index 1", func(t *testing.T) {
		id := resolveTabRef(client, srv.URL, "", "1")
		if id != "AAAA1111" {
			t.Errorf("expected AAAA1111, got %s", id)
		}
	})

	t.Run("numeric index 3", func(t *testing.T) {
		id := resolveTabRef(client, srv.URL, "", "3")
		if id != "CCCC3333" {
			t.Errorf("expected CCCC3333, got %s", id)
		}
	})

	t.Run("index out of range", func(t *testing.T) {
		id := resolveTabRef(client, srv.URL, "", "5")
		if id != "" {
			t.Errorf("expected empty, got %s", id)
		}
	})

	t.Run("index zero", func(t *testing.T) {
		id := resolveTabRef(client, srv.URL, "", "0")
		if id != "" {
			t.Errorf("expected empty, got %s", id)
		}
	})

	t.Run("negative index", func(t *testing.T) {
		// Negative numbers are valid ints but out of range
		id := resolveTabRef(client, srv.URL, "", "-1")
		if id != "" {
			t.Errorf("expected empty, got %s", id)
		}
	})

	t.Run("non-numeric passed through as ID", func(t *testing.T) {
		id := resolveTabRef(client, srv.URL, "", "AAAA1111")
		if id != "AAAA1111" {
			t.Errorf("expected AAAA1111, got %s", id)
		}
	})

	t.Run("hex-like ID passed through", func(t *testing.T) {
		id := resolveTabRef(client, srv.URL, "", "A1B2C3D4E5F6")
		if id != "A1B2C3D4E5F6" {
			t.Errorf("expected A1B2C3D4E5F6, got %s", id)
		}
	})
}

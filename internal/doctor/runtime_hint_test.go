package doctor

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestProbeRuntimeIsBestEffort(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Fatalf("path = %q, want /health", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatal(err)
	}
	hint := ProbeRuntime(context.Background(), &config.RuntimeConfig{Bind: host, Port: port})
	if !strings.Contains(hint, srv.URL) || !strings.Contains(hint, "/health") {
		t.Fatalf("ProbeRuntime() = %q, want runtime health guidance", hint)
	}

	if got := ProbeRuntime(context.Background(), &config.RuntimeConfig{}); got != "" {
		t.Fatalf("ProbeRuntime() without a server address = %q, want empty best-effort result", got)
	}
}

package orchestrator

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

func TestStopRegisteredBridgeRequiresEndpointExit(t *testing.T) {
	original := registeredBridgeStopTimeout
	registeredBridgeStopTimeout = 100 * time.Millisecond
	t.Cleanup(func() { registeredBridgeStopTimeout = original })

	for _, test := range []struct {
		name        string
		close       bool
		wantErr     bool
		wantPresent bool
	}{
		{name: "acknowledged but still alive", wantErr: true, wantPresent: true},
		{name: "acknowledged and exited", close: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/shutdown":
					w.WriteHeader(http.StatusOK)
					if test.close {
						go server.Close()
					}
				case "/health":
					w.WriteHeader(http.StatusOK)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()

			o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
			o.instances["registered"] = &InstanceInternal{
				Instance: bridge.Instance{
					ID: "registered", URL: server.URL, Status: "running",
					Attached: true, AttachType: "bridge",
				},
				URL: server.URL,
			}

			err := o.Stop("registered")
			if (err != nil) != test.wantErr {
				t.Fatalf("Stop error = %v, wantErr=%v", err, test.wantErr)
			}
			_, present := o.instances["registered"]
			if present != test.wantPresent {
				t.Fatalf("instance present=%v, want %v", present, test.wantPresent)
			}
		})
	}
}

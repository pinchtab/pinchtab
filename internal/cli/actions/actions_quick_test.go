package actions

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/pinchtab/pinchtab/internal/handlers"
)

// captureSnapshotQuery runs a CLI action against a stub server and returns the query it
// actually sent to /snapshot. Asserting on the emitted request rather than on the source is
// the point: `compact=true` read as a perfectly sensible line of code for as long as it
// survived, and only the wire shows that the server was never asked for a compact snapshot.
func captureSnapshotQuery(t *testing.T, run func(client *http.Client, base string)) url.Values {
	t.Helper()

	var got url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/snapshot" {
			got = r.URL.Query()
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte("e1:button\n"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"title":"probe","url":"http://example.test/"}`))
	}))
	defer server.Close()

	run(server.Client(), server.URL)
	if got == nil {
		t.Fatal("the action never called /snapshot")
	}
	return got
}

func TestQuickAsksForTheCompactSnapshotItIntendedTo(t *testing.T) {
	got := captureSnapshotQuery(t, func(client *http.Client, base string) {
		Quick(apiclient.Conn{Client: client, Base: base}, []string{"http://example.test/"})
	})

	if format := got.Get("format"); format != "compact" {
		t.Errorf("format = %q, want compact; `quick` was written to request the compact snapshot and sent an unread `compact=true` instead, so it always got the JSON one", format)
	}
	if filter := got.Get("filter"); filter != "interactive" {
		t.Errorf("filter = %q, want interactive", filter)
	}
	if got.Has("compact") {
		t.Errorf("still sending the parameter the server does not read: %v", got)
	}
}

// The general form of the same bug: any parameter the CLI sends that the server does not
// read is a flag that silently does nothing. Running the server's own validator over the
// emitted query catches the next one without anybody having to notice it.
func TestQuickSendsOnlyParametersTheServerHonours(t *testing.T) {
	got := captureSnapshotQuery(t, func(client *http.Client, base string) {
		Quick(apiclient.Conn{Client: client, Base: base}, []string{"http://example.test/"})
	})

	controls, err := handlers.ParseSnapshotCostControls(got)
	if err != nil {
		t.Fatalf("the server would reject the query this CLI action sends: %v", err)
	}
	if len(controls.Ignored) > 0 {
		t.Errorf("the server ignores %v of the parameters this action sends, so those flags do nothing", controls.Ignored)
	}
}

package apiclient

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/handlers"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

var printedRequestID = regexp.MustCompile(`\[requestId ([^\]]+)\]`)

// A real request-id middleware in front of a real activity recorder, so the id
// the CLI prints is the one the server minted and the one its access log holds.
func newCorrelatedFailingServer(t *testing.T) (*httptest.Server, *[]string, string) {
	t.Helper()
	logDir := t.TempDir()
	rec, err := activity.NewStore(logDir, 1)
	if err != nil {
		t.Fatal(err)
	}
	var served []string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		served = append(served, w.Header().Get(httpx.RequestIDHeader))
		httpx.Error(w, http.StatusBadRequest, errTabRequired)
	})
	srv := httptest.NewServer(handlers.RequestIDMiddleware(activity.Middleware(rec, "server", handler)))
	t.Cleanup(srv.Close)
	return srv, &served, logDir
}

type staticError string

func (e staticError) Error() string { return string(e) }

const errTabRequired = staticError("tab id required")

func printedID(t *testing.T, text string) string {
	t.Helper()
	m := printedRequestID.FindStringSubmatch(text)
	if m == nil {
		t.Fatalf("no request id printed in %q", text)
	}
	return m[1]
}

func activityLogMentions(t *testing.T, logDir, id string) bool {
	t.Helper()
	found := false
	err := filepath.WalkDir(logDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), `"requestId":"`+id+`"`) {
			found = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func TestTwoFailuresPrintTheirOwnServerRequestIDs(t *testing.T) {
	srv, served, logDir := newCorrelatedFailingServer(t)

	var ids []string
	for range 2 {
		_, err := DoGetRawE(srv.Client(), srv.URL, "", "/action", nil)
		if err == nil {
			t.Fatal("a 400 returned no error")
		}
		ids = append(ids, printedID(t, err.Error()))
	}
	if ids[0] == ids[1] {
		t.Fatalf("both failures printed %q; a constant is not a correlation id", ids[0])
	}
	if len(*served) != 2 || ids[0] != (*served)[0] || ids[1] != (*served)[1] {
		t.Fatalf("printed %v, server returned %v", ids, *served)
	}
	for _, id := range ids {
		if !activityLogMentions(t, logDir, id) {
			t.Errorf("printed id %q is not in the activity log, so the documented grep has no line to find", id)
		}
	}
}

func TestAFailureWithNoResponsePrintsNoRequestID(t *testing.T) {
	srv, _, _ := newCorrelatedFailingServer(t)
	if _, err := DoGetRawE(srv.Client(), srv.URL, "", "/action", nil); err == nil {
		t.Fatal("expected the priming failure to fail")
	}
	dead := httptest.NewServer(http.NotFoundHandler())
	base := dead.URL
	dead.Close()

	_, err := DoGetRawE(http.DefaultClient, base, "", "/action", nil)
	if err == nil {
		t.Fatal("a closed server answered")
	}
	if strings.Contains(err.Error(), "requestId") {
		t.Fatalf("a failure with no response printed an id: %q", err.Error())
	}
	if out := renderAPIErrorBody(500, []byte(`{"error":"x"}`)); strings.Contains(out, "requestId") {
		t.Fatalf("the previous response's id survived a failed request: %q", out)
	}
}

func TestSuccessfulOutputCarriesNoRequestID(t *testing.T) {
	srv := httptest.NewServer(handlers.RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})))
	defer srv.Close()
	body, err := DoGetRawE(srv.Client(), srv.URL, "", "/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "requestId") {
		t.Fatalf("success output changed: %s", body)
	}
}

func TestTheRequestIDHeaderStillMatchesWhatTheServerSends(t *testing.T) {
	if requestIDHeader != httpx.RequestIDHeader {
		t.Fatalf("CLI reads %q, server stamps %q", requestIDHeader, httpx.RequestIDHeader)
	}
}

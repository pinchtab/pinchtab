package mcp

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/pinchtab/pinchtab/internal/handlers"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

var toolRequestID = regexp.MustCompile(`\[requestId ([^\]]+)\]$`)

func correlatedServer(t *testing.T, status int, body string) (*httptest.Server, *[]string) {
	t.Helper()
	var served []string
	srv := httptest.NewServer(handlers.RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		served = append(served, w.Header().Get(httpx.RequestIDHeader))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})))
	t.Cleanup(srv.Close)
	return srv, &served
}

func toolID(t *testing.T, text string) string {
	t.Helper()
	m := toolRequestID.FindStringSubmatch(text)
	if m == nil {
		t.Fatalf("tool error carries no request id: %q", text)
	}
	return m[1]
}

func TestTwoToolFailuresCarryTheirOwnServerRequestIDs(t *testing.T) {
	srv, served := correlatedServer(t, http.StatusNotFound, `{"code":"ref_not_found","error":"ref e5 not found"}`)
	var ids []string
	for range 2 {
		res := callTool(t, "pinchtab_click", map[string]any{"ref": "e5"}, srv)
		if !res.IsError {
			t.Fatal("a 404 reached the agent as success")
		}
		text := resultText(t, res)
		if !strings.Contains(text, "ref e5 not found") {
			t.Fatalf("the body's reason was lost: %q", text)
		}
		ids = append(ids, toolID(t, text))
	}
	if ids[0] == ids[1] {
		t.Fatalf("both failures carry %q; a constant is not a correlation id", ids[0])
	}
	if len(*served) != 2 || ids[0] != (*served)[0] || ids[1] != (*served)[1] {
		t.Fatalf("tool errors carry %v, server returned %v", ids, *served)
	}
}

func TestAZeroSuccess200CarriesTheRequestIDToo(t *testing.T) {
	srv, served := correlatedServer(t, http.StatusOK, `{"successful":0,"failed":1,"total":1,"results":[{"kind":"click","error":"node gone"}]}`)
	res := callTool(t, "pinchtab_click", map[string]any{"ref": "e5"}, srv)
	if !res.IsError {
		t.Fatal("a zero-success 200 reached the agent as success")
	}
	if id := toolID(t, resultText(t, res)); id != (*served)[0] {
		t.Fatalf("tool error carries %q, server returned %q", id, (*served)[0])
	}
}

func TestAToolSuccessCarriesNoRequestID(t *testing.T) {
	srv, _ := correlatedServer(t, http.StatusOK, `{"successful":1,"failed":0,"total":1,"results":[{"kind":"click"}]}`)
	res := callTool(t, "pinchtab_click", map[string]any{"ref": "e5"}, srv)
	if res.IsError || strings.Contains(resultText(t, res), "requestId") {
		t.Fatalf("success output changed: %q", resultText(t, res))
	}
}

func TestAToolFailureWithNoResponseCarriesNoRequestID(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	c := NewClient(dead.URL, "")
	h := handlerMap(c)["pinchtab_click"]
	req := mcp.CallToolRequest{}
	req.Params.Name = "pinchtab_click"
	req.Params.Arguments = map[string]any{"ref": "e5"}
	res, err := h(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("a dead server answered")
	}
	if text := resultText(t, res); strings.Contains(text, "requestId") {
		t.Fatalf("a failure with no response carries an id: %q", text)
	}
}

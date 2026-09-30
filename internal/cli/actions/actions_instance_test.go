package actions

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
)

func TestInstanceNavigateOpensTheTargetURLInOneRequestAndPrintsItsResponse(t *testing.T) {
	m := newMockServer()
	defer m.close()
	m.setResponse(http.MethodPost, "/instances/inst_1e0877aa/tabs/open", http.StatusOK,
		`{"tabId":"7DB8EC9AFB64D61281CE4628ADDEF4CE","title":"Fixture Index","url":"http://127.0.0.1:18777/index.html"}`)

	out := captureStdout(t, func() {
		InstanceNavigate(apiclient.Conn{Client: http.DefaultClient, Base: m.base()}, []string{"inst_1e0877aa", "http://127.0.0.1:18777/index.html"})
	})

	if len(m.requests) != 1 {
		t.Fatalf("requests = %+v, want exactly one", m.requests)
	}
	req := m.requests[0]
	if req.Method != http.MethodPost || req.Path != "/instances/inst_1e0877aa/tabs/open" {
		t.Fatalf("request = %s %s, want POST /instances/inst_1e0877aa/tabs/open", req.Method, req.Path)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(req.Body), &sent); err != nil {
		t.Fatalf("decode request body %q: %v", req.Body, err)
	}
	if sent["url"] != "http://127.0.0.1:18777/index.html" {
		t.Fatalf("request body = %v, want url of the target page", sent)
	}
	for _, want := range []string{"7DB8EC9AFB64D61281CE4628ADDEF4CE", "Fixture Index", "http://127.0.0.1:18777/index.html"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output %q does not contain %q", out, want)
		}
	}
	if strings.Contains(out, "about:blank") {
		t.Fatalf("output %q still reports a blank tab", out)
	}
}

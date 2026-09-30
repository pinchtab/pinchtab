package actions

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

const extractTestSchema = `{"type":"object","required":["name","price"],"properties":{"name":{"type":"string","x-pinchtab-hint":"role:heading"},"price":{"type":"number","description":"product price"}}}`

const extractTestResponse = `{"data":{"name":"Sony WH-1000XM5","price":1299},"fields":{"name":{"ref":"e2","score":1,"confidence":"high","source":"hint"},"price":{"ref":"e3","score":0.71,"confidence":"medium","source":"text"}},"missing":[],"truncated":false,"latency_ms":3,"element_count":4}`

func newExtractCmd(schema string, set map[string]string) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("tab", "", "")
	cmd.Flags().String("schema", "", "")
	cmd.Flags().String("scope", "", "")
	cmd.Flags().Int("max-items", 0, "")
	cmd.Flags().Bool("fields", false, "")
	cmd.Flags().Bool("explain", false, "")
	cmd.Flags().Bool("json", false, "")
	_ = cmd.Flags().Set("schema", schema)
	for k, v := range set {
		_ = cmd.Flags().Set(k, v)
	}
	return cmd
}

func writeSchemaFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "product.schema.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write schema: %v", err)
	}
	return path
}

func sentBody(t *testing.T, m *mockServer) map[string]json.RawMessage {
	t.Helper()
	var body map[string]json.RawMessage
	if err := json.Unmarshal([]byte(m.lastBody), &body); err != nil {
		t.Fatalf("decode sent body %q: %v", m.lastBody, err)
	}
	return body
}

func TestExtractSendsTheSchemaFileUnchangedAndPrintsTypedData(t *testing.T) {
	m := newMockServer()
	m.response = extractTestResponse
	defer m.close()

	out := captureStdout(t, func() {
		Extract(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, newExtractCmd(writeSchemaFile(t, extractTestSchema+"\n"), nil))
	})

	if m.lastPath != "/extract" {
		t.Errorf("path = %s, want /extract", m.lastPath)
	}
	body := sentBody(t, m)
	if string(body["schema"]) != extractTestSchema {
		t.Errorf("schema sent as %s, want the file's JSON unchanged", body["schema"])
	}
	for _, absent := range []string{"scope", "maxItems", "tabId"} {
		if _, ok := body[absent]; ok {
			t.Errorf("body carries %q although no flag set it: %s", absent, m.lastBody)
		}
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		t.Fatalf("stdout is not the data object alone: %v\n%s", err, out)
	}
	if data["price"] != 1299.0 || data["name"] != "Sony WH-1000XM5" {
		t.Errorf("data = %#v", data)
	}
	if !strings.Contains(out, `"price": 1299`) {
		t.Errorf("price must print as a JSON number: %s", out)
	}
}

func TestExtractReadsTheSchemaFromStdinAndForwardsScopeMaxItemsAndTab(t *testing.T) {
	m := newMockServer()
	m.response = extractTestResponse
	defer m.close()

	cmd := newExtractCmd("-", map[string]string{"scope": "role:table", "max-items": "2", "tab": "tab1"})
	cmd.SetIn(strings.NewReader(extractTestSchema))
	captureStdout(t, func() { Extract(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, cmd) })

	if m.lastPath != "/tabs/tab1/extract" {
		t.Errorf("path = %s, want the tab route", m.lastPath)
	}
	body := sentBody(t, m)
	if string(body["schema"]) != extractTestSchema {
		t.Errorf("schema from stdin sent as %s", body["schema"])
	}
	if string(body["scope"]) != `"role:table"` || string(body["maxItems"]) != "2" {
		t.Errorf("scope/maxItems = %s/%s, want role:table/2", body["scope"], body["maxItems"])
	}
}

func TestExtractFieldsAppendsTheRefTableAndExplainAddsScores(t *testing.T) {
	for _, tc := range []struct {
		flag string
		want []string
	}{
		{"fields", []string{"name\te2\thigh", "price\te3\tmedium"}},
		{"explain", []string{"name\te2\thigh\t1.00\thint\t-", "price\te3\tmedium\t0.71\ttext\t-"}},
	} {
		t.Run(tc.flag, func(t *testing.T) {
			m := newMockServer()
			m.response = extractTestResponse
			defer m.close()

			out := captureStdout(t, func() {
				Extract(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, newExtractCmd(writeSchemaFile(t, extractTestSchema), map[string]string{tc.flag: "true"}))
			})
			for _, row := range tc.want {
				if !strings.Contains(out, row+"\n") {
					t.Errorf("output lacks row %q:\n%s", row, out)
				}
			}
			if _, ok := sentBody(t, m)["explain"]; ok {
				t.Errorf("--explain renders locally and must not be sent: %s", m.lastBody)
			}
		})
	}
}

func TestExtractFieldRowsListArrayItemsUnderTheirIndex(t *testing.T) {
	var fields map[string]any
	_ = json.Unmarshal([]byte(`{"products":{"ref":"e20","confidence":"high","items":[{"ref":"e21","fields":{"name":{"ref":"e22","confidence":"high"}}}]}}`), &fields)
	got := strings.Join(extractFieldRows(fields, "", false), "\n")
	want := "products\te20\thigh\nproducts[0]\te21\nproducts[0].name\te22\thigh"
	if got != want {
		t.Errorf("rows =\n%s\nwant\n%s", got, want)
	}
}

func TestReadExtractSchemaRefusesEmptyAndInvalidInputLocally(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"", "empty"},
		{"{not json", "not valid JSON"},
	} {
		_, err := readExtractSchema(schemaFromStdin, strings.NewReader(tc.input))
		if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "stdin") {
			t.Errorf("input %q: err = %v, want one naming stdin and %q", tc.input, err, tc.want)
		}
	}
	if _, err := readExtractSchema(filepath.Join(t.TempDir(), "absent.json"), nil); err == nil || !strings.Contains(err.Error(), "absent.json") {
		t.Errorf("missing file: err = %v, want one naming the path", err)
	}
}

func TestExtractCapturesTheVocabularySoTheNextClickEchoesIt(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	var clickVocab string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/extract") {
			w.Header().Set("X-PinchTab-Tab-Id", "T1")
			w.Header().Set("X-PinchTab-Vocab", "vocab-extract")
			_, _ = w.Write([]byte(extractTestResponse))
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		clickVocab, _ = body["vocab"].(string)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer srv.Close()

	captureStdout(t, func() {
		Extract(apiclient.Conn{Client: srv.Client(), Base: srv.URL}, newExtractCmd(writeSchemaFile(t, extractTestSchema), nil))
		Action(apiclient.Conn{Client: srv.Client(), Base: srv.URL}, "click", "e3", newActionCmd())
	})
	if clickVocab != "vocab-extract" {
		t.Errorf("click after extract echoed vocab %q, want the token extract returned", clickVocab)
	}
}

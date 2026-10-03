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

func TestExtractFieldsPrintsTheRefTableOnStderrAndKeepsStdoutPipeable(t *testing.T) {
	for _, tc := range []struct {
		flag string
		want []string
	}{
		{"fields", []string{"field\tref\tconfidence", "name\te2\thigh", "price\te3\tmedium"}},
		{"explain", []string{"field\tref\tconfidence\tscore\tsource\treason", "name\te2\thigh\t1.00\thint\t-", "price\te3\tmedium\t0.71\ttext\t-"}},
	} {
		t.Run(tc.flag, func(t *testing.T) {
			m := newMockServer()
			m.response = extractTestResponse
			defer m.close()

			var stdout string
			stderr := captureStderr(t, func() {
				stdout = captureStdout(t, func() {
					Extract(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, newExtractCmd(writeSchemaFile(t, extractTestSchema), map[string]string{tc.flag: "true"}))
				})
			})
			var data map[string]any
			if err := json.Unmarshal([]byte(stdout), &data); err != nil {
				t.Fatalf("stdout is not the data JSON alone: %v\n%s", err, stdout)
			}
			if want := strings.Join(tc.want, "\n") + "\n"; stderr != want {
				t.Errorf("stderr =\n%s\nwant\n%s", stderr, want)
			}
			if _, ok := sentBody(t, m)["explain"]; ok {
				t.Errorf("--explain renders locally and must not be sent: %s", m.lastBody)
			}
		})
	}
}

func TestExtractNamesEachUnmatchedPropertyOnStderrWithItsReason(t *testing.T) {
	const unmatched = `{"data":{"title":"Mugs"},"fields":{"title":{"ref":"e1","score":1,"confidence":"high","source":"text"},"items":{"ref":"e4","confidence":"low","reason":"no_field_match"},"brand":{"confidence":"low","reason":"no_match"}},"missing":[],"truncated":false}`
	for _, tc := range []struct {
		name, response, flag, wantStdout, wantStderr string
	}{
		{"empty data", `{"data":{},"fields":{"items":{"confidence":"low","reason":"no_field_match"}},"missing":[],"truncated":false}`, "", "{}\n", "unmatched: items (no_field_match); rerun with --explain\n"},
		{"one line per property, sorted", unmatched, "", "{\n  \"title\": \"Mugs\"\n}\n", "unmatched: brand (no_match); rerun with --explain\nunmatched: items (no_field_match); rerun with --explain\n"},
		{"fully matched", extractTestResponse, "", "", ""},
		{"explain already shows the reason", unmatched, "explain", "", "field\tref\tconfidence\tscore\tsource\treason\nbrand\t-\tlow\t0.00\t-\tno_match\nitems\te4\tlow\t0.00\t-\tno_field_match\ntitle\te1\thigh\t1.00\ttext\t-\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newMockServer()
			m.response = tc.response
			defer m.close()

			set := map[string]string{}
			if tc.flag != "" {
				set[tc.flag] = "true"
			}
			var stdout string
			stderr := captureStderr(t, func() {
				stdout = captureStdout(t, func() {
					Extract(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, newExtractCmd(writeSchemaFile(t, extractTestSchema), set))
				})
			})
			if stderr != tc.wantStderr {
				t.Errorf("stderr = %q, want %q", stderr, tc.wantStderr)
			}
			if tc.wantStdout != "" && stdout != tc.wantStdout {
				t.Errorf("stdout = %q, want the data object alone %q", stdout, tc.wantStdout)
			}
		})
	}
}

func TestExtractNamesEachUnmatchedNestedPathOnceWithItsItemCount(t *testing.T) {
	const partial = `{"data":{"products":[{"stock":"in stock","price":31},{"stock":"in stock"},{"stock":"in stock"}]},` +
		`"fields":{"products":{"ref":"e1","confidence":"high","items":[` +
		`{"ref":"e2","fields":{"name":{"confidence":"low","reason":"no_match"},"price":{"ref":"e3","confidence":"high"},"stock":{"ref":"e4","confidence":"high"}}},` +
		`{"ref":"e5","fields":{"name":{"confidence":"low","reason":"no_match"},"price":{"ref":"e6","confidence":"high","reason":"not_a_number"},"stock":{"ref":"e7","confidence":"high"}}},` +
		`{"ref":"e8","fields":{"name":{"confidence":"low","reason":"no_match"},"price":{"confidence":"low","reason":"no_match"},"stock":{"ref":"e9","confidence":"high"}}}]}},"missing":[],"truncated":false}`
	const deep = `{"data":{"products":[{"title":"Mug","variants":[{"size":"S"},{"size":"M","name":"Medium"}]}]},` +
		`"fields":{"products":{"ref":"e1","confidence":"high","items":[{"ref":"e2","fields":{"title":{"ref":"e3","confidence":"high"},"variants":{"ref":"e4","confidence":"high","items":[` +
		`{"ref":"e5","fields":{"size":{"ref":"e6","confidence":"high"},"name":{"confidence":"low","reason":"no_match"}}},` +
		`{"ref":"e7","fields":{"size":{"ref":"e8","confidence":"high"},"name":{"ref":"e9","confidence":"high"}}}]}}}]}},"missing":[],"truncated":false}`
	const matched = `{"data":{"products":[{"name":"Mug"},{"name":"Cup"}]},` +
		`"fields":{"products":{"ref":"e1","confidence":"high","items":[{"ref":"e2","fields":{"name":{"ref":"e3","confidence":"high"}}},{"ref":"e4","fields":{"name":{"ref":"e5","confidence":"high"}}}]}},"missing":[],"truncated":false}`
	for _, tc := range []struct {
		name, response, wantStderr string
	}{
		{"every item and some items, one line per path and reason", partial,
			"unmatched: products[].name (no_match in 3/3 items); rerun with --explain\n" +
				"unmatched: products[].price (no_match in 1/3 items); rerun with --explain\n" +
				"unmatched: products[].price (not_a_number in 1/3 items); rerun with --explain\n"},
		{"nested arrays at any depth", deep, "unmatched: products[].variants[].name (no_match in 1/2 items); rerun with --explain\n"},
		{"unmatched array prints only its own line", `{"data":{},"fields":{"products":{"confidence":"low","reason":"no_repeated_group"}},"missing":[],"truncated":false}`,
			"unmatched: products (no_repeated_group); rerun with --explain\n"},
		{"fully matched", matched, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newMockServer()
			m.response = tc.response
			defer m.close()

			var stdout string
			stderr := captureStderr(t, func() {
				stdout = captureStdout(t, func() {
					Extract(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, newExtractCmd(writeSchemaFile(t, extractTestSchema), nil))
				})
			})
			if stderr != tc.wantStderr {
				t.Errorf("stderr =\n%s\nwant\n%s", stderr, tc.wantStderr)
			}
			var envelope struct {
				Data json.RawMessage `json:"data"`
			}
			_ = json.Unmarshal([]byte(tc.response), &envelope)
			var got, want any
			_ = json.Unmarshal([]byte(stdout), &got)
			_ = json.Unmarshal(envelope.Data, &want)
			if gotJSON, wantJSON := mustJSON(t, got), mustJSON(t, want); gotJSON != wantJSON {
				t.Errorf("stdout = %s, want the data object unchanged %s", gotJSON, wantJSON)
			}
		})
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(raw)
}

func TestExtractFieldTableListsArrayItemsUnderTheirIndexWithEqualColumns(t *testing.T) {
	var fields map[string]any
	_ = json.Unmarshal([]byte(`{"products":{"ref":"e20","confidence":"high","items":[{"ref":"e21","fields":{"name":{"ref":"e22","confidence":"high"}}}]}}`), &fields)
	for _, tc := range []struct {
		explain bool
		want    string
	}{
		{false, "field\tref\tconfidence\nproducts\te20\thigh\nproducts[0]\te21\t-\nproducts[0].name\te22\thigh"},
		{true, "field\tref\tconfidence\tscore\tsource\treason\nproducts\te20\thigh\t0.00\t-\t-\nproducts[0]\te21\t-\t-\t-\t-\nproducts[0].name\te22\thigh\t0.00\t-\t-"},
	} {
		rows := extractFieldTable(fields, tc.explain)
		if got := strings.Join(rows, "\n"); got != tc.want {
			t.Errorf("explain=%v rows =\n%s\nwant\n%s", tc.explain, got, tc.want)
		}
		width := len(extractFieldColumns(tc.explain))
		for _, row := range rows {
			if n := len(strings.Split(row, "\t")); n != width {
				t.Errorf("explain=%v row %q has %d columns, want %d", tc.explain, row, n, width)
			}
		}
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

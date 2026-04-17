package actions

import (
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"
)

func newEvalCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("tab", "", "")
	cmd.Flags().Bool("await-promise", false, "")
	return cmd
}

func TestEvaluate(t *testing.T) {
	m := newMockServer()
	m.response = `{"result":"Example Domain"}`
	defer m.close()
	client := m.server.Client()

	cmd := newEvalCmd()
	Evaluate(client, m.base(), "", []string{"document.title"}, cmd)
	if m.lastPath != "/evaluate" {
		t.Errorf("expected /evaluate, got %s", m.lastPath)
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["expression"] != "document.title" {
		t.Errorf("expected expression=document.title, got %v", body["expression"])
	}
}

func TestEvaluateMultiWord(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newEvalCmd()
	Evaluate(client, m.base(), "", []string{"1", "+", "2"}, cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["expression"] != "1 + 2" {
		t.Errorf("expected expression='1 + 2', got %v", body["expression"])
	}
}

func TestEvaluateAwaitPromise(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newEvalCmd()
	_ = cmd.Flags().Set("await-promise", "true")
	Evaluate(client, m.base(), "", []string{"Promise.resolve(1)"}, cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["awaitPromise"] != true {
		t.Errorf("expected awaitPromise=true, got %v", body["awaitPromise"])
	}
}

func TestEvaluateAwaitPromiseOmittedByDefault(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newEvalCmd()
	Evaluate(client, m.base(), "", []string{"document.title"}, cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if _, present := body["awaitPromise"]; present {
		t.Errorf("expected awaitPromise to be omitted, got %v", body["awaitPromise"])
	}
}

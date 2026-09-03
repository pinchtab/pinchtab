package dashboard

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"

	"github.com/pinchtab/pinchtab/internal/authn"
)

const sparseUserConfigJSON = `{"server":{"port":"18799","token":"secret-token"}}`

func TestSavingOneTimeoutLeavesTheSecurityPostureByteIdentical(t *testing.T) {
	api := newConfigAPIOverFile(t, []byte(sparseUserConfigJSON))
	path := os.Getenv("PINCHTAB_CONFIG")
	fileBefore, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	before := runtimeSecurityInfo(api.cfg())
	if before.IDPIEnabled || len(before.AllowedDomains) != 0 {
		t.Fatalf("a sparse file booted with a policy it never stated: %+v", before)
	}

	getRes := httptest.NewRecorder()
	api.HandleGetConfig(getRes, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	if getRes.Code != http.StatusOK {
		t.Fatalf("HandleGetConfig() status = %d", getRes.Code)
	}
	body := putBodyFromGetPayloadWithActionTimeout(t, getRes, 45)

	putReq := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	putReq.AddCookie(&http.Cookie{Name: authn.CookieName, Value: "dashboard-session"})
	putRes := httptest.NewRecorder()
	api.HandlePutConfig(putRes, putReq)
	if putRes.Code != http.StatusOK {
		t.Fatalf("HandlePutConfig() status = %d: %s", putRes.Code, putRes.Body.String())
	}

	after := runtimeSecurityInfo(api.cfg())
	if !reflect.DeepEqual(before, after) {
		t.Errorf("saving a timeout changed the reported security posture\nbefore: %+v\nafter:  %+v", before, after)
	}
	env := decodeConfigEnvelope(t, putRes)
	if env.RestartRequired || len(env.RestartReasons) != 0 {
		t.Errorf("a save that changed no security field reports restartRequired=%v reasons=%v", env.RestartRequired, env.RestartReasons)
	}
	if env.Config.Timeouts.ActionSec != 45 {
		t.Errorf("the one field the caller changed did not land: %+v", env.Config.Timeouts)
	}
	fileAfter, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(fileAfter, []byte(`"actionSec": 45`)) || bytes.Contains(fileAfter, []byte(`"idpi"`)) {
		t.Errorf("the file gained something other than the edited timeout:\nbefore: %s\nafter:  %s", fileBefore, fileAfter)
	}
}

func putBodyFromGetPayloadWithActionTimeout(t *testing.T, getRes *httptest.ResponseRecorder, actionSec int) []byte {
	t.Helper()

	var envelope struct {
		Config map[string]any `json:"config"`
	}
	if err := json.Unmarshal(getRes.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("Unmarshal GET payload: %v", err)
	}
	timeouts, ok := envelope.Config["timeouts"].(map[string]any)
	if !ok {
		t.Fatalf("GET payload has no timeouts section: %v", envelope.Config)
	}
	timeouts["actionSec"] = actionSec
	body, err := json.Marshal(envelope.Config)
	if err != nil {
		t.Fatalf("Marshal PUT body: %v", err)
	}
	return body
}

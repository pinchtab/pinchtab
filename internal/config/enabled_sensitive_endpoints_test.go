package config

import (
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/routes"
)

func withCapability(t *testing.T, cap routes.Capability) *RuntimeConfig {
	t.Helper()
	meta, ok := routes.Meta(cap)
	if !ok {
		t.Fatalf("no metadata for %q", cap)
	}
	cfg := &RuntimeConfig{}
	field := reflect.ValueOf(cfg).Elem().FieldByName(capabilityFlagField(meta.Setting))
	if !field.IsValid() || field.Kind() != reflect.Bool {
		t.Fatalf("%s names no bool flag on RuntimeConfig; the reporter cannot see %q", meta.Setting, cap)
	}
	field.SetBool(true)
	return cfg
}

// The reporter used to be an if-chain over seven flags while the table held eight,
// so the eighth family reported as disabled while ten of its routes were live. Driven
// from the table: a capability added there reaches the reporter or this reds.
func TestEveryCapabilityInTheTableIsReportedWhenItsFlagIsOn(t *testing.T) {
	caps := routes.Capabilities()
	if len(caps) < 8 {
		t.Fatalf("table lists %d capabilities; the reporter guarded eight and this walk would prove less", len(caps))
	}
	for _, cap := range caps {
		meta, _ := routes.Meta(cap)
		got := withCapability(t, cap).EnabledSensitiveEndpoints()
		if len(got) != 1 || got[0] != meta.Label {
			t.Errorf("%s on reports %v, want exactly [%s]", meta.Setting, got, meta.Label)
		}
	}
	if got := (&RuntimeConfig{}).EnabledSensitiveEndpoints(); len(got) != 0 {
		t.Errorf("all flags off reports %v, want nothing", got)
	}
}

func TestAllowStateExportAloneIsNotReportedAsSensitiveEndpointsDisabled(t *testing.T) {
	cookiesOnly := (&RuntimeConfig{AllowCookies: true}).EnabledSensitiveEndpoints()
	stateOnly := (&RuntimeConfig{AllowStateExport: true}).EnabledSensitiveEndpoints()
	if len(cookiesOnly) != 1 || cookiesOnly[0] != "cookies" {
		t.Errorf("allowCookies alone reports %v", cookiesOnly)
	}
	if len(stateOnly) != 1 || stateOnly[0] != "stateExport" {
		t.Errorf("allowStateExport alone reports %v, want [stateExport]: ten live routes read as disabled", stateOnly)
	}
}

func TestTheReporterRestatesNoCapabilityCount(t *testing.T) {
	src, err := os.ReadFile("config_methods.go")
	if err != nil {
		t.Fatal(err)
	}
	if m := regexp.MustCompile(`make\(\[\]string, 0, \d+\)`).Find(src); m != nil {
		t.Errorf("config_methods.go restates the capability count: %s", m)
	}
	if strings.Count(string(src), "cfg.Allow") > 0 {
		t.Errorf("config_methods.go reads a capability flag by name; the routes table is the only list")
	}
}

package report

import (
	"os"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/config/workflow"
	"github.com/pinchtab/pinchtab/internal/routes"
)

// The recommended-defaults report used to hand-list its capability lines and had drifted
// from the canonical table, omitting cookies and stateExport, so it under-reported which
// capabilities a hardened posture turns off. The lines now derive from routes, so this
// census walks CapabilityEndpoints() and requires the report to name every gated capability
// with exactly the config path routes.Meta owns — an endpoint added without a table entry
// fails here rather than dropping out of the report silently.
func TestRecommendedDefaultsNameEveryCapability(t *testing.T) {
	gated := routes.CapabilityEndpoints()
	if len(gated) < 2 {
		t.Fatalf("only %d gated capabilities; this census would pass vacuously", len(gated))
	}

	have := map[string]bool{}
	for _, line := range capabilityDisableLines() {
		have[line] = true
	}

	for cap := range gated {
		meta, ok := routes.Meta(cap)
		if !ok {
			t.Errorf("capability %q gates routes but routes.Meta does not describe it, so the report cannot derive its line", cap)
			continue
		}
		want := meta.Setting + " = false"
		if !have[want] {
			t.Errorf("the recommended-defaults report omits %q for capability %q; it under-reports which capabilities are off", want, cap)
		}
	}
}

// No synthesis: every line the report emits must be a real capability's owned setting, not a
// fabricated path. Pairs with the coverage census so the guard cannot be satisfied by naming
// plausible-looking settings that no capability actually has.
func TestRecommendedDefaultsEmitNoFabricatedSetting(t *testing.T) {
	valid := map[string]bool{}
	for cap := range routes.CapabilityEndpoints() {
		if meta, ok := routes.Meta(cap); ok {
			valid[meta.Setting+" = false"] = true
		}
	}
	for _, line := range capabilityDisableLines() {
		if !valid[line] {
			t.Errorf("report emits %q, which no gated capability's routes.Meta owns", line)
		}
	}
}

// The block of capability lines appeared twice in this file — once in the ordered list and
// once in the sensitive-endpoints recommendation — and the two had already diverged. They now
// share capabilityDisableLines, so no capability disable line may be written as a literal:
// finding one means a hand-listed copy has returned.
func TestNoHandListedCapabilityDisableLineRemains(t *testing.T) {
	src, err := os.ReadFile("security_defaults.go")
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	text := string(src)
	found := 0
	for cap := range routes.CapabilityEndpoints() {
		meta, ok := routes.Meta(cap)
		if !ok {
			continue
		}
		if strings.Contains(text, `"`+meta.Setting+" = false"+`"`) {
			t.Errorf("security_defaults.go hand-lists %q as a literal; it must come from capabilityDisableLines so a ninth capability is covered by construction", meta.Setting+" = false")
		}
		found++
	}
	if found < 2 {
		t.Fatalf("checked only %d capabilities; this guard would prove little", found)
	}
}

func TestRecommendedDefaultsAreSatisfiedBySecurityUp(t *testing.T) {
	fc := config.DefaultFileConfig()
	workflow.ApplyRecommendedSecurityDefaults(&fc)
	if lines := RecommendedSecurityDefaultLines(config.NextRuntimeConfig(config.Load(), &fc)); len(lines) > 0 {
		t.Fatalf("RecommendedSecurityDefaultLines() = %v after security up; the report sends the operator back to a command that changes nothing", lines)
	}
}

func TestEveryRecommendedLineStatesTheValueSecurityUpWrites(t *testing.T) {
	relaxed := config.DefaultFileConfig()
	for cap := range routes.CapabilityEndpoints() {
		meta, _ := routes.Meta(cap)
		if err := config.SetConfigValue(&relaxed, meta.Setting, "true"); err != nil {
			t.Fatalf("set %s: %v", meta.Setting, err)
		}
	}
	relaxed.Server.Bind = "0.0.0.0"
	enabled := true
	relaxed.Security.Attach.Enabled = &enabled
	relaxed.Security.Attach.AllowHosts = []string{"chrome.internal"}
	relaxed.Security.IDPI = &config.IDPIConfig{}
	lines := RecommendedSecurityDefaultLines(config.NextRuntimeConfig(config.Load(), &relaxed))
	if len(lines) < 5 {
		t.Fatalf("RecommendedSecurityDefaultLines() = %v on a fully relaxed config; too few lines for this check to prove anything", lines)
	}
	hardened := config.DefaultFileConfig()
	workflow.ApplyRecommendedSecurityDefaults(&hardened)
	for _, line := range lines {
		path, value, ok := strings.Cut(line, " = ")
		if !ok {
			t.Errorf("line %q is not of the form <path> = <value>", line)
			continue
		}
		got, err := config.GetConfigValue(&hardened, path)
		if err != nil {
			t.Errorf("line %q names %s, which security up cannot address: %v", line, path, err)
			continue
		}
		if got != value {
			t.Errorf("line %q promises %s = %s but security up writes %q", line, path, value, got)
		}
	}
}

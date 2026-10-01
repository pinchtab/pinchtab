package scheduler

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/srccensus"
)

// New with an empty Config must land exactly on DefaultConfig. Anything else means a
// second set of literals has appeared inside New, which is how the defaults drifted
// out of reach of `config get` in the first place.
func TestNewFillsEveryKnobFromDefaultConfig(t *testing.T) {
	got := New(Config{}, nil, nil).cfg
	if want := DefaultConfig(); got != want {
		t.Errorf("New(Config{}).cfg = %+v, want DefaultConfig() %+v", got, want)
	}
}

// The operator-facing knobs come from internal/config, so `config get` and the running
// scheduler cannot disagree. Derived from the config package's own declaration rather
// than retyped here.
func TestDefaultConfigTakesTheOperatorKnobsFromTheConfigPackage(t *testing.T) {
	want := ConfigFromRuntime(config.DefaultSchedulerConfig())
	got := DefaultConfig()

	got.WatcherInterval = want.WatcherInterval
	if got != want {
		t.Errorf("DefaultConfig() operator knobs = %+v, want %+v from config.DefaultSchedulerConfig()", got, want)
	}
	if want.Strategy == "" || want.MaxQueueSize == 0 || want.ResultTTL == 0 {
		t.Fatalf("config.DefaultSchedulerConfig() looks empty (%+v), so this comparison proves nothing", want)
	}
}

// schedulerKnobFields are the settings an operator can address through config get/set.
// Their default values belong to internal/config/scheduler_defaults.go and nowhere
// else — a literal beside a second copy is exactly the drift this card paid off.
// Strategy is absent by name on purpose: internal/config also carries a
// MultiInstance.Strategy whose default is unrelated, so the scheduler's strategy is
// censused by its VALUE below instead.
var schedulerKnobFields = []string{
	"MaxQueueSize",
	"MaxPerAgent",
	"MaxInflight",
	"MaxPerAgentFlight",
	"ResultTTL",
	"ResultTTLSec",
	"WorkerCount",
	"MaxBatchSize",
}

var knobLiteralAssignment = regexp.MustCompile(`\b(` + strings.Join(schedulerKnobFields, "|") + `)\s*[:=]\s*("|\d|-\d)`)

func TestSchedulerDefaultsAreSpelledInOnePlace(t *testing.T) {
	owner := filepath.Join("..", "config", "scheduler_defaults.go")
	seen := map[string]int{}

	scopes := []struct {
		dir      string
		minFiles int
	}{
		{".", 8},
		{filepath.Join("..", "config"), 20},
		{filepath.Join("..", "server"), 5},
	}
	for _, scope := range scopes {
		for name, body := range srccensus.Load(t, scope.dir, scope.minFiles).Sources() {
			path := filepath.Join(scope.dir, name)
			for i, line := range strings.Split(body, "\n") {
				trimmed := strings.TrimSpace(line)
				field := ""
				if match := knobLiteralAssignment.FindStringSubmatch(trimmed); match != nil {
					field = match[1]
				}
				if strings.Contains(trimmed, `"`+config.DefaultSchedulerConfig().Strategy+`"`) {
					field = "Strategy"
				}
				if field == "" {
					continue
				}
				seen[field]++
				if path == owner {
					continue
				}
				t.Errorf("%s:%d gives %s a literal default; the one owner is %s, and a second copy is free to drift from what config get reports:\n\t%s",
					path, i+1, field, owner, trimmed)
			}
		}
	}

	for _, field := range append(schedulerKnobFields, "Strategy") {
		if field == "ResultTTL" {
			continue // the duration form is derived from ResultTTLSec, never written as a literal
		}
		if seen[field] == 0 {
			t.Errorf("found no literal assignment of %s anywhere; the owner has been renamed or the pattern no longer matches, which makes this census vacuous", field)
		}
	}
}

// Exposing the knob must not move the value it produces, and the value must arrive
// through the one conversion site rather than a second path. 50 is spelled out here
// rather than read from the config package: comparing two derived sides holds whatever
// both are changed to, and "the default is unchanged" is the claim being pinned.
func TestBatchSizeIsUnchangedAndComesThroughTheConversionSite(t *testing.T) {
	if got := DefaultConfig().MaxBatchSize; got != 50 {
		t.Errorf("default maxBatchSize = %d, want the 50 this package produced before the knob was exposed", got)
	}

	runtime := config.DefaultSchedulerConfig()
	runtime.MaxBatchSize = 5
	if got := ConfigFromRuntime(runtime).MaxBatchSize; got != 5 {
		t.Errorf("a configured maxBatchSize of 5 converted to %d; the conversion drops it, so config set writes a value nothing enforces", got)
	}
	if got := New(ConfigFromRuntime(runtime), nil, nil).cfg.MaxBatchSize; got != 5 {
		t.Errorf("the running scheduler enforces %d, want the configured 5", got)
	}
}

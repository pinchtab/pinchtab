package orchestrator

import (
	"sync"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/srccensus"
)

// orchWithConfig is the bare-struct orchestrator these tests want, with a config
// already published. The Live is a value field, so the zero orchestrator is
// usable and publishing is the only way in — there is no pointer for a test to
// keep and write through afterwards.
func orchWithConfig(cfg *config.RuntimeConfig) *Orchestrator {
	o := &Orchestrator{}
	o.LiveConfig().Publish(cfg)
	return o
}

// republishCfg is how a test changes a setting: it derives a new value and
// publishes it, the same discipline the dashboard save follows. Writing the
// fields of the published object instead would be the very race this package's
// accessors exist to close.
func republishCfg(o *Orchestrator, mutate func(*config.RuntimeConfig)) {
	next := config.CloneRuntimeConfig(o.cfg())
	if next == nil {
		next = &config.RuntimeConfig{}
	}
	mutate(next)
	o.LiveConfig().Publish(next)
}

// A publish is a whole-value swap, so one read of the published config is
// internally coherent: a reader never sees the token from one save and the
// evaluate policy from the next. The assertion cannot fail while the value is
// immutable — the test earns its place under -race, which is what reports the
// swap going back to a field-by-field write.
func TestOneReadOfThePublishedConfigIsCoherentUnderConcurrentPublishes(t *testing.T) {
	tokenFor := func(evaluate bool) string {
		if evaluate {
			return "evaluating"
		}
		return "refusing"
	}
	publish := func(evaluate bool) *config.RuntimeConfig {
		return &config.RuntimeConfig{
			Token:             tokenFor(evaluate),
			AllowEvaluate:     evaluate,
			InstancePortStart: 9868,
			InstancePortEnd:   9968,
		}
	}
	o := orchWithConfig(publish(false))

	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			cfg := o.cfg()
			if cfg.Token != tokenFor(cfg.AllowEvaluate) {
				t.Errorf("one read saw token %q with allowEvaluate=%v, which no published config carries", cfg.Token, cfg.AllowEvaluate)
				return
			}
		}
	}()

	for i := 0; i < 500; i++ {
		o.ApplyRuntimeConfig(publish(i%2 == 0))
	}
	close(stop)
	wg.Wait()
}

// The token and the evaluate policy are derived from the published config rather
// than copied into fields, so there is no second copy that can lag behind it.
func TestTheTokenAndEvaluatePolicyFollowTheAppliedConfig(t *testing.T) {
	o := orchWithConfig(nil)

	o.ApplyRuntimeConfig(&config.RuntimeConfig{Token: "first", AllowEvaluate: true})
	if o.cfgToken() != "first" || !o.AllowsEvaluate() {
		t.Fatalf("token=%q allowEvaluate=%v after the first apply", o.cfgToken(), o.AllowsEvaluate())
	}

	o.ApplyRuntimeConfig(&config.RuntimeConfig{Token: "second", AllowEvaluate: false})
	if o.cfgToken() != "second" || o.AllowsEvaluate() {
		t.Fatalf("token=%q allowEvaluate=%v after the second apply", o.cfgToken(), o.AllowsEvaluate())
	}

	o.ApplyRuntimeConfig(nil)
	if o.cfgToken() != "" || o.AllowsEvaluate() {
		t.Fatalf("token=%q allowEvaluate=%v after clearing the config", o.cfgToken(), o.AllowsEvaluate())
	}
}

// The accessors are only worth having if nothing goes around them. A new reader
// that touches o.live or o.portAllocator directly reads a value nobody
// synchronised, which is the defect this package was carrying — so the census
// names the file and line rather than waiting for -race to catch it on a lucky
// interleaving.
func TestThePublishedConfigAndThePortAllocatorAreOnlyTouchedByTheirAccessors(t *testing.T) {
	pkg := srccensus.Load(t, ".", 20)

	// The constructor sets both fields through composite-literal keys, which are not
	// selectors and so never appear here; the accessors are the whole population.
	owners := map[string][]string{
		"live":          {"cfg", "ApplyRuntimeConfig", "LiveConfig"},
		"portAllocator": {"ports", "SetPortRange"},
	}
	for field, ownerNames := range owners {
		spans := make(map[string]srccensus.Func, len(ownerNames))
		for _, name := range ownerNames {
			fn, ok := pkg.Func(name)
			if !ok {
				t.Fatalf("owner %s of o.%s no longer exists in %s; re-point this census at whatever replaced it", name, field, pkg.Dir())
			}
			spans[name] = fn
		}

		sites := pkg.FieldReferences(field)
		if len(sites) < len(ownerNames) {
			t.Fatalf("only %d reference(s) to o.%s for %d accessor(s); the census is matching almost nothing and would pass vacuously", len(sites), field, len(ownerNames))
		}
		covered := map[string]bool{}
		for _, site := range sites {
			owned := ""
			for name, span := range spans {
				if pkg.Contains(span, site) {
					owned = name
					break
				}
			}
			covered[owned] = true
			if owned == "" {
				t.Errorf("%s touches o.%s outside its accessor; route it through the accessor so the value it reads is the published one", site, field)
			}
		}
		for _, name := range ownerNames {
			if !covered[name] {
				t.Errorf("%s no longer touches o.%s, so it has stopped being an accessor for it and this census is guarding one site fewer than it claims", name, field)
			}
		}
	}
}

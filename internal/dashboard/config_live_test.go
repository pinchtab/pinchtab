package dashboard

import (
	"time"

	"github.com/pinchtab/pinchtab/internal/browsersession"
	"github.com/pinchtab/pinchtab/internal/config"
)

// The APIs hold the shared publication point, not a config pointer. Tests that
// only care about one config wrap it in a Live of its own here rather than
// spelling the wrapper at every call.
func newConfigAPIForTest(
	runtime *config.RuntimeConfig,
	instances InstanceLister,
	profiles profileLister,
	applier runtimeConfigApplier,
	agents agentCounter,
	version string,
	startedAt time.Time,
) *ConfigAPI {
	return NewConfigAPI(config.NewLive(runtime), instances, profiles, applier, agents, version, startedAt)
}

func newAuthAPIForTest(runtime *config.RuntimeConfig, sessions *browsersession.Manager) *AuthAPI {
	return NewAuthAPI(config.NewLive(runtime), sessions)
}

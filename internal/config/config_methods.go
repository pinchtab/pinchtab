package config

import (
	"fmt"
	"github.com/pinchtab/pinchtab/internal/routes"
	"path/filepath"
	"reflect"
	"strings"
)

// EnabledSensitiveEndpoints returns the names of sensitive endpoint families
// that are currently enabled in the runtime configuration.
func (cfg *RuntimeConfig) EnabledSensitiveEndpoints() []string {
	if cfg == nil {
		return nil
	}
	caps := routes.Capabilities()
	enabled := make([]string, 0, len(caps))
	for _, cap := range caps {
		if cfg.CapabilityEnabled(cap) {
			meta, _ := routes.Meta(cap)
			enabled = append(enabled, meta.Label)
		}
	}
	return enabled
}

// CapabilityEnabled reads the flag a capability's declared setting names, so the
// routes table is the only place a capability is listed.
func (cfg *RuntimeConfig) CapabilityEnabled(cap routes.Capability) bool {
	meta, ok := routes.Meta(cap)
	if !ok || cfg == nil {
		return false
	}
	field := reflect.ValueOf(cfg).Elem().FieldByName(capabilityFlagField(meta.Setting))
	return field.IsValid() && field.Kind() == reflect.Bool && field.Bool()
}

func capabilityFlagField(setting string) string {
	name := strings.TrimPrefix(setting, "security.")
	return strings.ToUpper(name[:1]) + name[1:]
}

// ActivityStateDir returns the directory root used for activity log storage.
// When unset, activity logs live under the main server state directory.
func (cfg *RuntimeConfig) ActivityStateDir() string {
	if cfg == nil {
		return ""
	}
	return cfg.StateDir
}

// ActivityLogDir is where the activity store keeps its files. This package owns the
// state-directory layout — the same rule as profiles.baseDir — so the store is handed
// the directory rather than appending its own name to a parent, and `config get` can
// report it without a second join.
func (cfg *RuntimeConfig) ActivityLogDir() string {
	root := cfg.ActivityStateDir()
	if root == "" {
		return ""
	}
	return filepath.Join(root, activityLogSubdir)
}

const activityLogSubdir = "activity"

// activityStateDirReason is the fact both surfaces below state, written once. A user
// reaching for observability.activity.stateDir wants the logs elsewhere and deserves to
// learn why that is not on offer, rather than being told only that the key is wrong.
const activityStateDirReason = "activity logs are always written to <server.stateDir>/" + activityLogSubdir +
	" so two instances cannot share a log directory"

// ActivityStateDirRefusal is what `config set` answers. It ends in an action because there
// is one: server.stateDir moves the logs, and `config set server.stateDir <path>` performs
// it.
const ActivityStateDirRefusal = "observability.activity.stateDir is not settable: " + activityStateDirReason +
	"; set server.stateDir to move them"

// ActivityStateDirAdvisory is what a FILE already carrying the key is told, and it
// instructs nothing on purpose. The key is inert, so there is nothing the reader must do —
// and the wording it replaces ("remove the key") named an action no shipped command
// performs, while riding a diagnostic that aborted every later config write off a TTY.
const ActivityStateDirAdvisory = "observability.activity.stateDir is ignored: " + activityStateDirReason

// ProxyServerRequiredAdvisory reports a proxy block that carries credentials, a bypass
// list or geo overrides but no server: the values are kept and PinchTab uses none of
// them, because with no server there is no proxy. It takes the block's own path so the
// key it tells the reader to set is the one that block actually needs — browser.proxy
// and each browser.targets.<name>.proxy have their own.
//
// An ADVISORY and not a validation error, deliberately, and the reason is the path
// rather than the severity: validation errors gate `config set` through a confirm that
// answers no off a TTY, so making this one gate would refuse the very write that
// completes the block — `config set browser.proxy.username` would abort for every agent
// and pipe, which is the failure the advisory tier was introduced to end. Unlike the
// other advisory here there IS an action, so this one names it.
func ProxyServerRequiredAdvisory(field string) string {
	return fmt.Sprintf("%s has no server, so nothing it holds is used: set %s.server to a scheme://host:port to make it take effect", field, field)
}

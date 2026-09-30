package config

import "fmt"

type intRule func(n int) string

func minInt(min int) intRule {
	return func(n int) string {
		if n < min {
			return fmt.Sprintf("must be >= %d (got %d)", min, n)
		}
		return ""
	}
}

func positiveInt(n int) string {
	if n <= 0 {
		return fmt.Sprintf("must be > 0 (got %d)", n)
	}
	return ""
}

func intBetween(min, max int) intRule {
	return func(n int) string {
		if n < min || n > max {
			return fmt.Sprintf("must be between %d and %d (got %d)", min, max, n)
		}
		return ""
	}
}

var intRules = map[string]intRule{
	"server.networkBufferSize":         intBetween(1, MaxNetworkBufferSize),
	"server.retainNetworkBodyMaxBytes": intBetween(0, MaxRetainNetworkBodyMaxBytes),

	"multiInstance.restart.maxRestarts": func(n int) string {
		if n < -1 {
			return fmt.Sprintf("must be >= 0 or -1 for unlimited (got %d)", n)
		}
		return ""
	},
	"multiInstance.restart.initBackoffSec": minInt(1),
	"multiInstance.restart.maxBackoffSec":  minInt(1),
	"multiInstance.restart.stableAfterSec": minInt(1),

	"instanceDefaults.tabPolicy.closeDelaySec": minInt(0),
	"instanceDefaults.maxTabs":                 minInt(1),
	"instanceDefaults.maxParallelTabs":         minInt(0),

	"security.idpi.shieldThreshold":         intBetween(0, 100),
	"security.downloadMaxBytes":             intBetween(1, MaxDownloadMaxBytes),
	"security.memorySnapshotMaxBytes":       intBetween(1, MaxMemorySnapshotMaxBytes),
	"security.uploadMaxRequestBytes":        intBetween(1, MaxUploadMaxRequestBytes),
	"security.uploadMaxFiles":               intBetween(1, MaxUploadMaxFiles),
	"security.uploadMaxFileBytes":           intBetween(1, MaxUploadMaxFileBytes),
	"security.uploadMaxTotalBytes":          intBetween(1, MaxUploadMaxTotalBytes),
	"timeouts.actionSec":                    minInt(0),
	"timeouts.navigateSec":                  minInt(0),
	"timeouts.shutdownSec":                  minInt(0),
	"timeouts.waitNavMs":                    minInt(0),
	"autoSolver.maxAttempts":                minInt(1),
	"autoSolver.solverTimeoutSec":           positiveInt,
	"autoSolver.retryBaseDelayMs":           minInt(0),
	"autoSolver.retryMaxDelayMs":            minInt(0),
	"observability.activity.sessionIdleSec": minInt(0),
	"observability.activity.retentionDays":  positiveInt,
	"sessions.dashboard.idleTimeoutSec":     positiveInt,
	"sessions.dashboard.maxLifetimeSec":     positiveInt,
	"sessions.dashboard.elevationWindowSec": positiveInt,

	"profiles.quarantineKeep": func(n int) string {
		if n < 0 {
			return fmt.Sprintf("must be >= 0, 0 keeps every quarantined profile (got %d)", n)
		}
		return ""
	},
}

func checkIntRule(path string, n int) error {
	rule, ok := intRules[path]
	if !ok {
		return nil
	}
	if msg := rule(n); msg != "" {
		return ValidationError{Field: path, Message: msg}
	}
	return nil
}

func appendIntRule(errs []error, path string, n int) []error {
	if err := checkIntRule(path, n); err != nil {
		return append(errs, err)
	}
	return errs
}

func appendIntPtrRule(errs []error, path string, n *int) []error {
	if n == nil {
		return errs
	}
	return appendIntRule(errs, path, *n)
}

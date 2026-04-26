package config

import (
	"path/filepath"
)

// CurrentConfigVersion is bumped when config schema changes require migration or wizard re-run.
const CurrentConfigVersion = "0.8.0"

// DefaultFileConfig returns a FileConfig with sensible defaults (nested format).
func DefaultFileConfig() FileConfig {
	start := 9868
	end := 9968
	restartMaxRestarts := 20
	restartInitBackoffSec := 2
	restartMaxBackoffSec := 60
	restartStableAfterSec := 300
	maxTabs := 20
	allowEvaluate := false
	allowMacro := false
	allowScreencast := false
	allowDownload := false
	downloadMaxBytes := DefaultDownloadMaxBytes
	allowUpload := false
	allowClipboard := false
	allowStateExport := false
	enableActionGuards := true
	uploadMaxRequestBytes := DefaultUploadMaxRequestBytes
	uploadMaxFiles := DefaultUploadMaxFiles
	uploadMaxFileBytes := DefaultUploadMaxFileBytes
	uploadMaxTotalBytes := DefaultUploadMaxTotalBytes
	maxRedirects := -1
	attachEnabled := false
	activityEnabled := true
	activitySessionIdleSec := 1800
	activityRetentionDays := 30
	activityDashboardEvents := false
	activityServerEvents := false
	activityBridgeEvents := false
	activityOrchestratorEvents := false
	activitySchedulerEvents := false
	activityMCPEvents := false
	activityOtherEvents := false
	dashboardSessionPersist := true
	dashboardSessionIdleSec := 7 * 24 * 60 * 60
	dashboardSessionMaxLifetimeSec := 7 * 24 * 60 * 60
	dashboardSessionElevationWindowSec := 15 * 60
	dashboardSessionPersistElevationAcrossRestart := false
	dashboardSessionRequireElevation := false
	autoSolverEnabled := false
	autoSolverAutoTrigger := true
	autoSolverTriggerOnNavigate := true
	autoSolverTriggerOnAction := true
	autoSolverMaxAttempts := 8
	autoSolverSolverTimeoutSec := 30
	autoSolverRetryBaseDelayMs := 500
	autoSolverRetryMaxDelayMs := 10000
	autoSolverLLMFallback := false
	return FileConfig{
		ConfigVersion: CurrentConfigVersion,
		Server: ServerConfig{
			Port:     defaultPort,
			Bind:     "127.0.0.1",
			StateDir: userConfigDir(),
		},
		Browser: BrowserConfig{
			ChromeVersion:  "144.0.7559.133",
			ExtensionPaths: []string{defaultExtensionsDir(userConfigDir())},
		},
		InstanceDefaults: InstanceDefaultsConfig{
			Mode:              "headless",
			MaxTabs:           &maxTabs,
			StealthLevel:      "light",
			TabEvictionPolicy: "close_lru",
		},
		Security: SecurityConfig{
			AllowEvaluate:          &allowEvaluate,
			AllowMacro:             &allowMacro,
			AllowScreencast:        &allowScreencast,
			AllowDownload:          &allowDownload,
			AllowedDomains:         append([]string(nil), defaultLocalAllowedDomains...),
			DownloadAllowedDomains: []string{},
			DownloadMaxBytes:       &downloadMaxBytes,
			AllowUpload:            &allowUpload,
			AllowClipboard:         &allowClipboard,
			AllowStateExport:       &allowStateExport,
			EnableActionGuards:     &enableActionGuards,
			UploadMaxRequestBytes:  &uploadMaxRequestBytes,
			UploadMaxFiles:         &uploadMaxFiles,
			UploadMaxFileBytes:     &uploadMaxFileBytes,
			UploadMaxTotalBytes:    &uploadMaxTotalBytes,
			MaxRedirects:           &maxRedirects,
			Attach: AttachConfig{
				Enabled:      &attachEnabled,
				AllowHosts:   []string{"127.0.0.1", "localhost", "::1"},
				AllowSchemes: []string{"ws", "wss"},
			},
			IDPI: IDPIConfig{
				Enabled:        true,
				StrictMode:     true,
				ScanContent:    true,
				WrapContent:    true,
				ScanTimeoutSec: 5,
			},
		},
		Profiles: ProfilesConfig{
			BaseDir:        filepath.Join(userConfigDir(), "profiles"),
			DefaultProfile: "default",
		},
		MultiInstance: MultiInstanceConfig{
			Strategy:          "always-on",
			AllocationPolicy:  "fcfs",
			InstancePortStart: &start,
			InstancePortEnd:   &end,
			Restart: MultiInstanceRestartConfig{
				MaxRestarts:    &restartMaxRestarts,
				InitBackoffSec: &restartInitBackoffSec,
				MaxBackoffSec:  &restartMaxBackoffSec,
				StableAfterSec: &restartStableAfterSec,
			},
		},
		Timeouts: TimeoutsConfig{
			ActionSec:   30,
			NavigateSec: 60,
			ShutdownSec: 10,
			WaitNavMs:   1000,
		},
		Observability: ObservabilityFileConfig{
			Activity: ActivityFileConfig{
				Enabled:        &activityEnabled,
				SessionIdleSec: &activitySessionIdleSec,
				RetentionDays:  &activityRetentionDays,
				StateDir:       "",
				Events: ActivityEventsFileConfig{
					Dashboard:    &activityDashboardEvents,
					Server:       &activityServerEvents,
					Bridge:       &activityBridgeEvents,
					Orchestrator: &activityOrchestratorEvents,
					Scheduler:    &activitySchedulerEvents,
					MCP:          &activityMCPEvents,
					Other:        &activityOtherEvents,
				},
			},
		},
		Sessions: SessionsFileConfig{
			Dashboard: DashboardSessionFileConfig{
				Persist:                       &dashboardSessionPersist,
				IdleTimeoutSec:                &dashboardSessionIdleSec,
				MaxLifetimeSec:                &dashboardSessionMaxLifetimeSec,
				ElevationWindowSec:            &dashboardSessionElevationWindowSec,
				PersistElevationAcrossRestart: &dashboardSessionPersistElevationAcrossRestart,
				RequireElevation:              &dashboardSessionRequireElevation,
			},
		},
		AutoSolver: AutoSolverFileConfig{
			Enabled:           &autoSolverEnabled,
			AutoTrigger:       &autoSolverAutoTrigger,
			TriggerOnNavigate: &autoSolverTriggerOnNavigate,
			TriggerOnAction:   &autoSolverTriggerOnAction,
			MaxAttempts:       &autoSolverMaxAttempts,
			SolverTimeoutSec:  &autoSolverSolverTimeoutSec,
			RetryBaseDelayMs:  &autoSolverRetryBaseDelayMs,
			RetryMaxDelayMs:   &autoSolverRetryMaxDelayMs,
			Solvers:           []string{"cloudflare", "semantic", "capsolver", "twocaptcha"},
			LLMFallback:       &autoSolverLLMFallback,
		},
	}
}

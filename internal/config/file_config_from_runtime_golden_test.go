package config

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite the FileConfigFromRuntime golden")

const populatedFileConfigJSON = `{
  "server": {"port": "9911", "bind": "0.0.0.0", "token": "tok", "stateDir": "/tmp/state", "logLevel": "debug",
             "networkBufferSize": 250, "retainNetworkBodies": true, "retainNetworkBodyMaxBytes": 4096,
             "trustProxyHeaders": true, "cookieSecure": true},
  "browsers": {"default": "cloak", "available": ["chrome", "cloak"]},
  "browser": {"browserVersion": "131", "browserBinary": "/usr/bin/chromium", "browserDebugPort": 9333,
              "browserExtraFlags": "--flag-a", "extensionPaths": ["/ext/a", "/ext/b"],
              "proxy": {"server": "http://proxy:3128", "username": "u", "password": "p", "bypass": ["localhost"]},
              "cloak": {"level": "high"},
              "defaultTarget": "primary", "fallbackOrder": ["primary", "backup"],
              "targets": {"primary": {"provider": "chrome"}, "backup": {"provider": "cloak"}}},
  "instanceDefaults": {"mode": "headed", "noRestore": true, "timezone": "Europe/Rome", "blockImages": true,
                       "blockMedia": true, "blockAds": true, "maxTabs": 7, "maxParallelTabs": 3,
                       "userAgent": "ua/1", "noAnimations": true, "captureAllowActivation": true, "humanize": true,
                       "stealthLevel": "max", "tabEvictionPolicy": "reject", "dialogAutoAccept": true,
                       "tabPolicy": {"allowedDomains": ["a.example"], "blockedDomains": ["b.example"]}},
  "security": {"allowEvaluate": true, "allowMacro": true, "allowScreencast": true, "allowDownload": true,
               "allowCookies": true, "allowNetworkIntercept": true, "allowMemory": true, "allowFileScheme": true,
               "allowedDomains": [], "downloadAllowedDomains": ["dl.example"], "downloadMaxBytes": 123456, "memorySnapshotMaxBytes": 654321,
               "allowUpload": true, "allowClipboard": true, "allowStateExport": true,
               "uploadMaxRequestBytes": 1000, "uploadMaxFiles": 4, "uploadMaxFileBytes": 500, "uploadMaxTotalBytes": 900,
               "maxRedirects": 9, "trustedProxyCIDRs": ["10.0.0.0/8"], "trustedResolveCIDRs": ["192.168.0.0/16"],
               "trustLoopbackProxy": true,
               "attach": {"enabled": true, "allowHosts": ["attach.example"], "allowSchemes": ["ws"], "forwardProxyAuth": true},
               "idpi": {"enabled": true, "strictMode": true, "scanContent": true, "wrapContent": true,
                        "customPatterns": ["ignore previous"], "scanTimeoutSec": 3, "shieldThreshold": 55}},
  "profiles": {"baseDir": "/tmp/profiles", "defaultProfile": "main", "quarantineKeep": 2},
  "multiInstance": {"strategy": "always-on", "instancePortStart": 9900, "instancePortEnd": 9950,
                    "restart": {"maxRestarts": 5, "initBackoffSec": 2, "maxBackoffSec": 60, "stableAfterSec": 120}},
  "timeouts": {"actionSec": 11, "navigateSec": 22, "shutdownSec": 33, "waitNavMs": 444},
  "observability": {"activity": {"enabled": true, "sessionIdleSec": 300, "retentionDays": 9,
                                  "events": {"dashboard": true, "server": true, "bridge": true, "orchestrator": true,
                                             "scheduler": true, "mcp": true, "other": true}}},
  "sessions": {"dashboard": {"persist": true, "idleTimeoutSec": 600, "maxLifetimeSec": 7200, "elevationWindowSec": 90,
                             "persistElevationAcrossRestart": true, "requireElevation": true},
               "agent": {"enabled": true, "mode": "strict", "idleTimeoutSec": 120, "maxLifetimeSec": 3600}},
  "autoSolver": {"enabled": true, "autoTrigger": true, "triggerOnNavigate": true, "triggerOnAction": true,
                 "maxAttempts": 4, "solverTimeoutSec": 30, "retryBaseDelayMs": 100, "retryMaxDelayMs": 900,
                 "solvers": ["capsolver", "2captcha"], "llmProvider": "anthropic", "llmFallback": true,
                 "external": {"capsolverKey": "ck", "twoCaptchaKey": "tk"},
                 "credentials": {"login": {"user": "lu", "password": "lp"},
                                 "signup": {"name": "sn", "email": "se", "password": "sp"},
                                 "form": {"field1": "f1", "field2": "f2", "email": "fe"}}}
}`

func populatedRuntimeConfig(t *testing.T) *RuntimeConfig {
	t.Helper()
	var fc FileConfig
	if err := json.Unmarshal([]byte(populatedFileConfigJSON), &fc); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	cfg := &RuntimeConfig{}
	applyFileConfig(cfg, &fc)
	if len(cfg.IDPI.CustomPatterns) == 0 || !cfg.TrustProxyHeaders || len(cfg.AttachAllowHosts) == 0 {
		t.Fatalf("fixture did not populate the fields the golden exists to pin: %+v", cfg)
	}
	return cfg
}

func TestFileConfigFromRuntimeEmitsTheRecordedJSON(t *testing.T) {
	got, err := json.MarshalIndent(FileConfigFromRuntime(populatedRuntimeConfig(t)), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", "file_config_from_runtime.golden.json")
	if *updateGolden {
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v (run with -update to record it)", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FileConfigFromRuntime JSON drifted from %s; diff it against the marshalled output before deciding whether to re-record:\n%s", path, got)
	}
}

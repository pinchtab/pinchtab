package orchestrator

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/profiles"
	internalurls "github.com/pinchtab/pinchtab/internal/urls"
)

// attachExternalInstance registers an external instance or updates an existing
// bridge in place (upsert). Non-bridge duplicates still return an error.
func (o *Orchestrator) attachExternalInstance(name string, inst bridge.Instance, authToken string) (*bridge.Instance, bool, error) {
	o.mu.Lock()
	for _, existing := range o.instances {
		if existing.ProfileName == name && instanceIsActive(existing) {
			if existing.Attached && inst.AttachType == "bridge" && existing.AttachType == "bridge" {
				if existing.authToken != "" && subtle.ConstantTimeCompare([]byte(existing.authToken), []byte(authToken)) != 1 {
					o.mu.Unlock()
					return nil, false, fmt.Errorf("bridge %q already attached: token mismatch", name)
				}
				existing.URL = inst.URL
				existing.Instance.URL = inst.URL
				existing.authToken = authToken
				existing.Status = "running"
				existing.Error = ""
				existing.StartTime = time.Now()
				if inst.Browser != "" {
					existing.Browser = inst.Browser
				}
				result := existing.Instance
				o.mu.Unlock()

				o.syncInstanceToManager(&result)
				return &result, false, nil
			}
			o.mu.Unlock()
			return nil, false, fmt.Errorf("instance with name %q already exists", name)
		}
	}
	o.mu.Unlock()

	profileID := o.idMgr.ProfileID(name)
	instanceID := o.idMgr.InstanceID(profileID, name)
	inst.ID = instanceID
	inst.ProfileID = profileID
	inst.ProfileName = name
	inst.Status = "running"
	inst.StartTime = time.Now()
	internal := &InstanceInternal{
		Instance:  inst,
		URL:       inst.URL,
		authToken: authToken,
	}

	o.mu.Lock()
	o.instances[instanceID] = internal
	o.mu.Unlock()

	o.syncInstanceToManager(&internal.Instance)
	return &internal.Instance, true, nil
}

// Attach wraps an externally-managed browser via a child bridge; the external process is never killed on Stop.
func (o *Orchestrator) Attach(name, cdpURL string) (*bridge.Instance, error) {
	return o.AttachWithProvider(name, cdpURL, "")
}

func (o *Orchestrator) AttachWithProvider(name, cdpURL, provider string) (*bridge.Instance, error) {
	return o.AttachWithOptions(name, cdpURL, AttachOptions{Browser: provider})
}

func (o *Orchestrator) AttachWithOptions(name, cdpURL string, opts AttachOptions) (*bridge.Instance, error) {
	if err := profiles.ValidateProfileName(name); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cdpURL) == "" {
		return nil, fmt.Errorf("cdpUrl is required")
	}
	if err := o.validateAttachURL(cdpURL); err != nil {
		return nil, err
	}
	resolved, err := o.resolveAttachOptions(opts, config.BrowserChrome)
	if err != nil {
		return nil, err
	}

	bridgePort, err := o.portAllocator.AllocatePort()
	if err != nil {
		return nil, fmt.Errorf("failed to allocate bridge port: %w", err)
	}
	released := false
	defer func() {
		if !released {
			o.portAllocator.ReleasePort(bridgePort)
		}
	}()
	portStr := strconv.Itoa(bridgePort)

	profileID := o.idMgr.ProfileID(name)
	instanceID := o.idMgr.InstanceID(profileID, name)

	o.mu.Lock()
	for _, existing := range o.instances {
		if existing.ProfileName == name && instanceIsActive(existing) {
			o.mu.Unlock()
			return nil, fmt.Errorf("instance with name %q already exists", name)
		}
	}
	// Reserve the name before releasing the lock: the spawn below is slow and
	// a concurrent same-name attach would otherwise pass the scan above too
	// and start a second child process against the same external browser.
	// instanceIsActive treats cmd-less "starting" records as active.
	o.instances[instanceID] = &InstanceInternal{
		Instance: bridge.Instance{
			ID:          instanceID,
			ProfileID:   profileID,
			ProfileName: name,
			Status:      "starting",
			Attached:    true,
		},
	}
	o.mu.Unlock()
	reserved := true
	defer func() {
		if reserved {
			o.mu.Lock()
			delete(o.instances, instanceID)
			o.mu.Unlock()
		}
	}()

	// RemoteCDPURL/RemoteBrowserName are passed via CLI flags rather than persisted.
	stateDir := filepath.Join(o.baseDir, "attach", instanceID)
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return nil, fmt.Errorf("create attach state dir: %w", err)
	}
	if err := os.Chmod(stateDir, 0700); err != nil {
		return nil, fmt.Errorf("set attach state dir permissions: %w", err)
	}
	childConfigPath, err := o.writeAttachChildConfig(portStr, resolved.Browser, stateDir)
	if err != nil {
		return nil, fmt.Errorf("write attach child config: %w", err)
	}

	envOverrides := map[string]string{
		"PINCHTAB_PORT":   portStr,
		"PINCHTAB_CONFIG": childConfigPath,
	}
	if o.internalToken != "" {
		envOverrides["PINCHTAB_INTERNAL_TOKEN"] = o.internalToken
	}
	env := mergeEnvWithOverrides(filterEnvWithPrefixes(os.Environ(), "PINCHTAB_"), envOverrides)

	logBuf := newRingBuffer(256 * 1024)
	args := []string{
		"bridge",
		"--cdp-attach", cdpURL,
		"--browser", resolved.Browser,
		"--remote-browser-name", name,
	}
	slog.Info("starting CDP attach bridge child",
		"id", instanceID, "name", name, "port", portStr, "provider", resolved.Browser,
		"cdpUrl", internalurls.RedactForLog(cdpURL),
	)
	cmd, err := o.runner.Run(context.Background(), o.binary, args, env, logBuf, logBuf)
	if err != nil {
		return nil, fmt.Errorf("failed to start CDP attach bridge: %w", err)
	}

	baseURL := o.childInstanceBaseURL(portStr)
	internal := &InstanceInternal{
		Instance: bridge.Instance{
			ID:          instanceID,
			ProfileID:   profileID,
			ProfileName: name,
			Port:        portStr,
			URL:         baseURL,
			Mode:        bridge.ModeFromHeadless(false),
			Headless:    false,
			Status:      "starting",
			StartTime:   time.Now(),
			Attached:    true,
			AttachType:  "cdp-bridge",
			CdpURL:      cdpURL,
			Browser:     resolved.Browser,
		},
		URL:    baseURL,
		cmd:    cmd,
		logBuf: logBuf,

		browser: resolved.Browser,
	}

	o.mu.Lock()
	o.instances[instanceID] = internal
	o.mu.Unlock()
	reserved = false
	released = true

	go o.monitor(internal)

	healthTimeout := o.attachHealthCheckTimeout
	if healthTimeout <= 0 {
		healthTimeout = 15 * time.Second
	}
	if err := waitForChildBridgeHealthyFunc(o, internal, healthTimeout); err != nil {
		tail := tailChildLog(logBuf, 20)
		slog.Warn("CDP attach bridge did not become healthy in time; tearing down child bridge", "id", instanceID, "err", err, "childLogTail", tail)
		if stopErr := o.Stop(instanceID); stopErr != nil {
			slog.Warn("failed to tear down unhealthy CDP attach bridge", "id", instanceID, "err", stopErr)
		}
		if tail != "" {
			return nil, fmt.Errorf("CDP attach bridge did not become healthy: %w; child log tail:\n%s", err, tail)
		}
		return nil, fmt.Errorf("CDP attach bridge did not become healthy: %w", err)
	}

	result := internal.Instance
	slog.Info("attached external browser via CDP bridge",
		"id", result.ID, "name", name, "provider", resolved.Browser,
		"url", internalurls.RedactForLog(result.URL),
		"cdpUrl", internalurls.RedactForLog(cdpURL),
	)
	o.emitEvent("instance.attached", &result)
	return &result, nil
}

func (o *Orchestrator) waitForChildBridgeHealthy(inst *InstanceInternal, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	healthURL := strings.TrimRight(inst.URL, "/") + "/health"
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest(http.MethodGet, healthURL, nil)
		o.applyInstanceAuth(req, inst)
		resp, err := o.client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				o.mu.Lock()
				// Only promote starting -> running. The concurrent monitor()
				// goroutine may have already moved the instance to a terminal
				// state (error on process exit, or stopping/stopped); a transient
				// health 200 must not resurrect it.
				if inst.Status == "starting" {
					inst.Status = "running"
				}
				o.mu.Unlock()
				return nil
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("child bridge health check timed out after %v", timeout)
}

// AttachBridge registers an already-running bridge server as an attached instance.
// If a bridge with the same name is already attached, it is updated in place (upsert)
// provided the caller presents the current bridge token.
func (o *Orchestrator) AttachBridge(name, baseURL, token string) (*bridge.Instance, bool, error) {
	return o.AttachBridgeWithOptions(name, baseURL, token, AttachOptions{})
}

func (o *Orchestrator) AttachBridgeWithOptions(name, baseURL, token string, opts AttachOptions) (*bridge.Instance, bool, error) {
	if err := o.validateAttachURL(baseURL); err != nil {
		return nil, false, err
	}
	preserveExistingBrowser := strings.TrimSpace(opts.Browser) == "" &&
		o.hasActiveAttachedBridge(name)
	resolved, err := o.resolveAttachOptions(opts, "")
	if err != nil {
		return nil, false, err
	}
	if preserveExistingBrowser {
		resolved.Browser = ""
	}

	normalizedBaseURL := strings.TrimRight(baseURL, "/")
	if parsed, err := url.Parse(normalizedBaseURL); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		normalizedBaseURL = parsed.Scheme + "://" + parsed.Host
	}

	inst, created, err := o.attachExternalInstance(name, bridge.Instance{
		Attached:   true,
		AttachType: "bridge",
		URL:        normalizedBaseURL,
		Mode:       bridge.ModeFromHeadless(false),
		Browser:    resolved.Browser,
	}, token)
	if err != nil {
		return nil, false, err
	}

	slog.Info("attached to external bridge",
		"id", inst.ID, "name", name, "provider", inst.Browser,
		"url", internalurls.RedactForLog(inst.URL),
	)
	o.emitEvent("instance.attached", inst)
	if created {
		o.mu.RLock()
		internal := o.instances[inst.ID]
		o.mu.RUnlock()
		if internal != nil {
			go o.monitorAttachedBridge(internal)
		}
	}
	return inst, created, nil
}

func (o *Orchestrator) hasActiveAttachedBridge(name string) bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	for _, existing := range o.instances {
		if existing.ProfileName == name && instanceIsActive(existing) && existing.Attached && existing.AttachType == "bridge" {
			return true
		}
	}
	return false
}

func (o *Orchestrator) resolveAttachOptions(opts AttachOptions, defaultProvider string) (AttachOptions, error) {
	requestedProvider := strings.TrimSpace(opts.Browser)

	var configured []string
	if o.runtimeCfg != nil {
		configured = o.runtimeCfg.BrowsersAvailable
	}

	explicitProvider := requestedProvider != ""
	parsedProvider := ""
	if explicitProvider {
		provider, err := config.ParseBrowser(requestedProvider, configured)
		if err != nil {
			return AttachOptions{}, err
		}
		parsedProvider = provider
	}

	if o.runtimeCfg == nil || len(o.runtimeCfg.Targets) == 0 {
		if parsedProvider == "" && defaultProvider != "" {
			provider, err := config.ParseBrowser(defaultProvider, configured)
			if err != nil {
				return AttachOptions{}, err
			}
			parsedProvider = provider
		}
		return AttachOptions{Browser: parsedProvider}, nil
	}

	if parsedProvider == "" {
		resolved, err := config.ResolveDefaultBrowserTarget(o.runtimeCfg)
		if err != nil {
			return AttachOptions{}, err
		}
		if resolved != nil && !resolved.Legacy && resolved.Provider != "" {
			parsedProvider = resolved.Provider
		}
	}
	if parsedProvider == "" && defaultProvider != "" {
		provider, err := config.ParseBrowser(defaultProvider, configured)
		if err != nil {
			return AttachOptions{}, err
		}
		parsedProvider = provider
	}
	if parsedProvider == "" {
		parsedProvider = config.BrowserChrome
	}
	return AttachOptions{Browser: parsedProvider}, nil
}

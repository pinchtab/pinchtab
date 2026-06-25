package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/profiles"
)

func (o *Orchestrator) Launch(name, port string, headless bool, extensionPaths []string) (*bridge.Instance, error) {
	opts := LaunchOptions{
		ExtensionPaths: extensionPaths,
	}
	return o.LaunchWithTargetSelection(name, port, headless, "", nil, opts)
}

func (o *Orchestrator) LaunchWithOptions(name, port string, headless bool, opts LaunchOptions) (*bridge.Instance, error) {
	// Validate profile name to prevent path traversal attacks
	if err := profiles.ValidateProfileName(name); err != nil {
		return nil, err
	}
	reservedPorts := make([]int, 0, 2)
	defer func() {
		for _, reserved := range reservedPorts {
			o.portAllocator.ReleasePort(reserved)
		}
	}()

	o.mu.Lock()

	if port == "" || port == "0" {
		o.mu.Unlock()
		allocatedPort, err := o.portAllocator.AllocatePort()
		if err != nil {
			return nil, fmt.Errorf("failed to allocate port: %w", err)
		}
		port = fmt.Sprintf("%d", allocatedPort)
		reservedPorts = append(reservedPorts, allocatedPort)
		o.mu.Lock()
	} else {
		o.mu.Unlock()
		portInt, err := parsePortNumber(port)
		if err != nil {
			return nil, err
		}
		port = strconv.Itoa(portInt)
		if err := o.portAllocator.ReservePort(portInt); err != nil {
			return nil, fmt.Errorf("failed to reserve port %s: %w", port, err)
		}
		if portInt >= o.portAllocator.start && portInt <= o.portAllocator.end {
			reservedPorts = append(reservedPorts, portInt)
		}
		o.mu.Lock()
	}

	for _, inst := range o.instances {
		if inst.Port == port && instanceIsActive(inst) {
			o.mu.Unlock()
			return nil, fmt.Errorf("port %s already in use by instance %q", port, inst.ProfileName)
		}
		if inst.ProfileName == name && instanceIsActive(inst) {
			o.mu.Unlock()
			return nil, fmt.Errorf("profile %q already has an active instance (%s)", name, inst.Status)
		}
	}
	portInspection := o.runner.InspectPort(port)
	if !portInspection.Available {
		o.mu.Unlock()
		err := portConflictError(port, portInspection)
		slog.Error("instance launch blocked by port conflict", "profile", name, "port", port, "pid", portInspection.PID, "command", portInspection.Command, "error", err.Error())
		return nil, err
	}

	profileID := o.idMgr.ProfileID(name)
	instanceID := o.idMgr.InstanceID(profileID, name)

	if inst, ok := o.instances[instanceID]; ok && inst.Status == "running" {
		o.mu.Unlock()
		return nil, fmt.Errorf("instance already running for profile %q", name)
	}

	o.mu.Unlock()

	cdpPort, err := o.portAllocator.AllocatePort()
	if err != nil {
		return nil, fmt.Errorf("failed to allocate browser debug port: %w", err)
	}
	reservedPorts = append(reservedPorts, cdpPort)

	profilePath := filepath.Join(o.baseDir, name)
	if o.profiles != nil {
		if resolvedPath, err := o.profiles.ProfilePath(name); err == nil {
			profilePath = resolvedPath
		}
	}
	if err := os.MkdirAll(filepath.Join(profilePath, "Default"), 0755); err != nil {
		return nil, fmt.Errorf("create profile dir: %w", err)
	}
	instanceStateDir := filepath.Join(profilePath, ".pinchtab-state")
	if err := os.MkdirAll(instanceStateDir, 0700); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}
	if err := os.Chmod(instanceStateDir, 0700); err != nil {
		return nil, fmt.Errorf("set state dir permissions: %w", err)
	}

	requestedPolicy := cloneSecurityPolicy(opts.SecurityPolicy)
	effectivePolicy := effectiveSecurityPolicy(o.runtimeCfg, requestedPolicy)

	effectiveCfg := o.runtimeCfg
	targetPromoted := false
	// A resolved target name is authoritative: re-deriving from the provider
	// picks the wrong target when several targets share one provider.
	if targetName := strings.TrimSpace(opts.TargetName); targetName != "" && o.runtimeCfg != nil && len(o.runtimeCfg.Targets) > 0 {
		resolved, err := config.ResolveExplicitBrowserTarget(o.runtimeCfg, targetName)
		if err == nil {
			effectiveCfg = resolved.Config
			targetPromoted = true
		} else {
			slog.Warn("launch: resolved target name no longer resolves; falling back to provider-derived config", "target", targetName, "err", err)
		}
	}
	if browser := strings.TrimSpace(opts.Browser); !targetPromoted && browser != "" && o.runtimeCfg != nil && len(o.runtimeCfg.Targets) > 0 {
		// Lenient: only promote a target when the provider maps to an unambiguous
		// winner (single match, or the configured default among several); an
		// ambiguous/zero match leaves the provider-derived config in place.
		if target, _ := config.MatchBrowserToTarget(o.runtimeCfg, browser); target != "" {
			if resolved, err := config.ResolveExplicitBrowserTarget(o.runtimeCfg, target); err == nil {
				effectiveCfg = resolved.Config
			}
		}
	}

	childConfigPath, err := o.writeChildConfig(effectiveCfg, port, cdpPort, profilePath, instanceStateDir, headless, opts.ExtensionPaths, effectivePolicy)
	if err != nil {
		return nil, fmt.Errorf("write child config: %w", err)
	}

	envOverrides := map[string]string{
		"PINCHTAB_PORT":   port,
		"PINCHTAB_CONFIG": childConfigPath,
	}
	if o.internalToken != "" {
		envOverrides["PINCHTAB_INTERNAL_TOKEN"] = o.internalToken
	}
	env := mergeEnvWithOverrides(filterEnvWithPrefixes(os.Environ(), "PINCHTAB_"), envOverrides)

	if opts.Browser != "" {
		var configured []string
		if o.runtimeCfg != nil {
			configured = o.runtimeCfg.BrowsersAvailable
		}
		if _, err := config.ParseBrowser(opts.Browser, configured); err != nil {
			return nil, fmt.Errorf("invalid browser %q: %w", opts.Browser, err)
		}
	}

	logBuf := newRingBuffer(256 * 1024)
	slog.Info("starting instance process", "id", instanceID, "profile", name, "port", port)

	cmd, err := o.runner.Run(context.Background(), o.binary, []string{"bridge"}, env, logBuf, logBuf)
	if err != nil {
		return nil, fmt.Errorf("failed to start: %w", err)
	}

	browser := opts.Browser
	if browser == "" && o.runtimeCfg != nil {
		browser = config.NormalizeBrowser(o.runtimeCfg.DefaultBrowser)
	}

	inst := &InstanceInternal{
		Instance: bridge.Instance{
			ID:             instanceID,
			ProfileID:      profileID,
			ProfileName:    name,
			Port:           port,
			URL:            o.childInstanceBaseURL(port),
			Mode:           bridge.ModeFromHeadless(headless),
			Headless:       headless,
			Status:         "starting",
			StartTime:      time.Now(),
			SecurityPolicy: effectivePolicy,
			Browser:        browser,
		},
		URL:     o.childInstanceBaseURL(port),
		cdpPort: cdpPort,
		cmd:     cmd,
		logBuf:  logBuf,

		requestedSecurityPolicy: requestedPolicy,
		requestedProvider:       opts.RequestedProvider,
		browser:                 opts.Browser,
		effectiveBinary:         effectiveBinaryFromCfg(effectiveCfg),
	}

	o.mu.Lock()
	o.instances[instanceID] = inst
	o.mu.Unlock()
	reservedPorts = nil

	go o.monitor(inst)

	return &inst.Instance, nil
}

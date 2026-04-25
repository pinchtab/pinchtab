package report

import (
	"fmt"
	"github.com/pinchtab/pinchtab/internal/config"
)

func HandleConfigShow(cfg *config.RuntimeConfig) {
	fmt.Println(styleStdout(headingStyle, "Current configuration (env > file > defaults):"))
	fmt.Println()
	fmt.Println(styleStdout(headingStyle, "Server"))
	fmt.Printf("  Port:           %s\n", cfg.Port)
	fmt.Printf("  Bind:           %s\n", cfg.Bind)
	fmt.Printf("  Token:          %s\n", config.MaskToken(cfg.Token))
	fmt.Printf("  State Dir:      %s\n", cfg.StateDir)
	fmt.Printf("  Instance Ports: %d-%d\n", cfg.InstancePortStart, cfg.InstancePortEnd)
	fmt.Println()
	fmt.Println(styleStdout(headingStyle, "Security"))
	fmt.Printf("  Evaluate:       %v\n", cfg.AllowEvaluate)
	fmt.Printf("  Macro:          %v\n", cfg.AllowMacro)
	fmt.Printf("  Screencast:     %v\n", cfg.AllowScreencast)
	fmt.Printf("  Download:       %v\n", cfg.AllowDownload)
	fmt.Printf("  Upload:         %v\n", cfg.AllowUpload)
	fmt.Println()
	fmt.Println(styleStdout(headingStyle, "Browser / Instance Defaults"))
	fmt.Printf("  Headless:       %v\n", cfg.Headless)
	fmt.Printf("  No Restore:     %v\n", cfg.NoRestore)
	fmt.Printf("  Profile Dir:    %s\n", cfg.ProfileDir)
	fmt.Printf("  Profiles Dir:   %s\n", cfg.ProfilesBaseDir)
	fmt.Printf("  Default Profile: %s\n", cfg.DefaultProfile)
	fmt.Printf("  Max Tabs:       %d\n", cfg.MaxTabs)
	fmt.Printf("  Stealth:        %s\n", cfg.StealthLevel)
	fmt.Printf("  Tab Eviction:   %s\n", cfg.TabEvictionPolicy)
	if cfg.TabLifecyclePolicy == "close_idle" {
		fmt.Printf("  Tab Lifecycle:  %s (%s)\n", cfg.TabLifecyclePolicy, cfg.TabCloseDelay)
	} else {
		fmt.Printf("  Tab Lifecycle:  %s\n", cfg.TabLifecyclePolicy)
	}
	fmt.Printf("  Extensions:     %v\n", cfg.ExtensionPaths)
	fmt.Println()
	fmt.Println(styleStdout(headingStyle, "Multi-Instance"))
	fmt.Printf("  Strategy:       %s\n", cfg.Strategy)
	fmt.Printf("  Allocation:     %s\n", cfg.AllocationPolicy)
	fmt.Printf("  Max Restarts:   %d\n", cfg.RestartMaxRestarts)
	fmt.Printf("  Init Backoff:   %v\n", cfg.RestartInitBackoff)
	fmt.Printf("  Max Backoff:    %v\n", cfg.RestartMaxBackoff)
	fmt.Printf("  Stable After:   %v\n", cfg.RestartStableAfter)
	fmt.Println()
	fmt.Println(styleStdout(headingStyle, "Attach"))
	fmt.Printf("  Enabled:        %v\n", cfg.AttachEnabled)
	fmt.Printf("  Allow Hosts:    %v\n", cfg.AttachAllowHosts)
	fmt.Printf("  Allow Schemes:  %v\n", cfg.AttachAllowSchemes)
	fmt.Println()
	fmt.Println(styleStdout(headingStyle, "Timeouts"))
	fmt.Printf("  Action:         %v\n", cfg.ActionTimeout)
	fmt.Printf("  Navigate:       %v\n", cfg.NavigateTimeout)
	fmt.Printf("  Shutdown:       %v\n", cfg.ShutdownTimeout)
}

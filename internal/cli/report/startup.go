package report

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/pinchtab/pinchtab/internal/cli/termstyle"
	"github.com/pinchtab/pinchtab/internal/config"
)

var menuBannerOnce sync.Once

type StartupBannerOptions struct {
	Mode         string
	ListenAddr   string
	ListenStatus string
	PublicURL    string
	Strategy     string
	Allocation   string
	ProfileDir   string
}

func PrintStartupBanner(cfg *config.RuntimeConfig, opts StartupBannerOptions) {
	if opts.Mode == "menu" {
		printed := false
		menuBannerOnce.Do(func() {
			printed = true
		})
		if !printed {
			return
		}
	}

	writeBannerLine(renderStartupLogo(blankIfEmpty(opts.Mode, "server")))
	writeBannerf("  %s  %s\n", styleLabel("listen"), formatListenValue(blankIfEmpty(opts.ListenAddr, cfg.ListenAddr()), defaultListenStatus(opts.Mode, opts.ListenStatus)))
	if opts.PublicURL != "" {
		writeBannerf("  %s  %s\n", styleLabel("url"), styleValue(opts.PublicURL))
	}
	strat := blankIfEmpty(opts.Strategy, "manual")
	alloc := blankIfEmpty(opts.Allocation, "none")
	writeBannerf("  %s  %s\n", styleLabel("str,plc"), styleValue(fmt.Sprintf("%s,%s", strat, alloc)))

	daemonStatus := styleStdout(warningStyle, "not installed")
	if IsDaemonInstalled() {
		daemonStatus = styleStdout(successStyle, "ok")
	}
	writeBannerf("  %s  %s\n", styleLabel("daemon"), daemonStatus)

	posture := AssessSecurityPosture(cfg)
	writeBannerf("  %s  %s  %s\n", styleLabel("security"), styleSecurityBar(posture.Level, posture.Bar), styleSecurityLevel(posture.Level))

	if opts.ProfileDir != "" {
		writeBannerf("  %s  %s\n", styleLabel("profile"), styleValue(opts.ProfileDir))
	}
	writeBannerLine("")
}

func PrintSecuritySummary(w io.Writer, cfg *config.RuntimeConfig, prefix string, detailed bool) {
	posture := AssessSecurityPosture(cfg)
	if detailed {
		writeSummaryf(w, "%s%s %s  %s\n", prefix, styleHeading("Security"), styleSecurityBar(posture.Level, posture.Bar), styleSecurityLevel(posture.Level))
		for _, check := range posture.Checks {
			writeSummaryf(w, "%s  %s %s %s\n", prefix, styleMarker(check.Passed), styleCheckLabel(check.Label), styleCheckDetail(check.Passed, check.Detail))
		}
	} else {
		writeSummaryf(w, "%s%s  %s  %s\n", prefix, styleLabel("security"), styleSecurityBar(posture.Level, posture.Bar), styleSecurityLevel(posture.Level))
	}
}

func blankIfEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func renderStartupLogo(mode string) string {
	if mode == "menu" || mode == "" {
		return styleLogo(startupLogo)
	}
	return styleLogo(startupLogo) + "  " + styleMode(mode)
}

func writeBannerLine(line string) {
	_, _ = fmt.Fprintln(os.Stdout, line)
}

func writeBannerf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stdout, format, args...)
}

func writeSummaryf(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

func styleHeading(text string) string {
	return applyStyle(text, headingStyle)
}

func styleLogo(text string) string {
	return applyStyle(text, headingStyle)
}

func styleMode(text string) string {
	return applyStyle(text, commandStyle)
}

func styleLabel(text string) string {
	return applyStyle(fmt.Sprintf("%-8s", text), mutedStyle)
}

func styleValue(text string) string {
	return applyStyle(text, valueStyle)
}

func styleListenStatus(status string) string {
	switch status {
	case "running":
		return applyStyle(status, successStyle)
	case "starting":
		return applyStyle(status, warningStyle)
	case "stopped":
		return applyStyle(status, mutedStyle)
	default:
		return applyStyle(status, valueStyle)
	}
}

func styleCheckLabel(text string) string {
	return applyStyle(fmt.Sprintf("%-20s", text), mutedStyle)
}

func styleCheckDetail(passed bool, text string) string {
	if passed {
		return applyStyle(text, successStyle)
	}
	return applyStyle(text, warningStyle)
}

func styleMarker(passed bool) string {
	if passed {
		return applyStyle("ok", successStyle)
	}
	return applyStyle("!!", errorStyle)
}

func styleSecurityLevel(level string) string {
	return applyStyle(level, termstyle.NewStyle().Foreground(termstyle.Color(SecurityLevelColor(level))).Bold(true))
}

func styleSecurityBar(level, bar string) string {
	return applyStyle(bar, termstyle.NewStyle().Foreground(termstyle.Color(SecurityLevelColor(level))).Bold(true))
}

func SecurityLevelColor(level string) string {
	switch level {
	case "LOCKED":
		return string(colorSuccess)
	case "GUARDED":
		return string(colorWarning)
	case "ELEVATED":
		return string(colorWarning)
	default:
		return string(colorDanger)
	}
}

func applyStyle(text string, style termstyle.Style) string {
	return styleStdout(style, text)
}

func formatListenValue(addr, status string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		return styleValue(addr)
	}
	return fmt.Sprintf("%-8s %s", styleListenStatus(status), styleValue(addr))
}

func defaultListenStatus(mode, explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}
	switch mode {
	case "menu":
		return "stopped"
	case "server", "bridge", "mcp":
		return "starting"
	default:
		return ""
	}
}

const startupLogo = `   ____  _            _     _____     _
  |  _ \(_)_ __   ___| |__ |_   _|_ _| |__
  | |_) | | '_ \ / __| '_ \  | |/ _  | '_ \
  |  __/| | | | | (__| | | | | | (_| | |_) |
  |_|   |_|_| |_|\___|_| |_| |_|\__,_|_.__/`

//go:build windows

package bridge

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const findPIDsByPowerShellScript = `$needle = $env:PINCHTAB_NEEDLE
if ([string]::IsNullOrEmpty($needle)) { exit 0 }
Get-CimInstance Win32_Process -Filter "Name='chrome.exe'" |
Where-Object { $_.CommandLine -and $_.CommandLine.Contains($needle) } |
Select-Object -ExpandProperty ProcessId`

func findPIDsByPowerShell(needle string) []int {
	cmd := exec.Command("powershell", "-NoProfile", "-Command", findPIDsByPowerShellScript)
	cmd.Env = append(os.Environ(), "PINCHTAB_NEEDLE="+needle)

	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var pids []int
	for _, line := range bytes.Split(out, []byte{'\n'}) {
		pidStr := strings.TrimSpace(string(line))
		if pidStr == "" {
			continue
		}
		pid, err := strconv.Atoi(pidStr)
		if err != nil || pid <= 0 {
			continue
		}
		pids = append(pids, pid)
	}
	return pids
}

const parentPIDsPowerShellScript = `Get-CimInstance Win32_Process |
ForEach-Object { "$($_.ProcessId) $($_.ParentProcessId)" }`

func parentPIDs() map[int]int {
	out, err := exec.Command("powershell", "-NoProfile", "-Command", parentPIDsPowerShellScript).Output()
	if err != nil {
		return nil
	}
	return parsePIDPairs(out)
}

func forceKillPIDs(pids []int) int {
	killed := 0
	for _, pid := range pids {
		// #nosec G204 -- pid is an int, not user-controlled string
		cmd := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid))
		if cmd.Run() == nil {
			killed++
			slog.Info("cleanup: killed chrome process", "pid", pid)
		}
	}
	return killed
}

func findChromePIDsByProfileDir(profileDir string) []int {
	return findPIDsByPowerShell(fmt.Sprintf("--user-data-dir=%s", profileDir))
}

func killChromeByProfileDir(profileDir string) int {
	pids := findChromePIDsByProfileDirFunc(profileDir)
	if len(pids) == 0 {
		return 0
	}
	return forceKillPIDs(pids)
}

// taskkill without /F (no SIGKILL escalation).
func terminateChromeByProfileDir(profileDir string) int {
	pids := findChromePIDsByProfileDirFunc(profileDir)
	if len(pids) == 0 {
		return 0
	}
	for _, pid := range pids {
		// #nosec G204 -- pid is an int, not user-controlled string
		cmd := exec.Command("taskkill", "/PID", strconv.Itoa(pid))
		if err := cmd.Run(); err == nil {
			slog.Info("cleanup: taskkill chrome process", "pid", pid)
		}
	}
	return len(pids)
}

// Spares Chrome that a live peer server launched.
func KillAllPinchtabChrome() int {
	var matched []int
	seen := make(map[int]bool)

	for _, needle := range []string{"pinchtab-profile", ".pinchtab\\profiles"} {
		for _, pid := range findPIDsByPowerShell(needle) {
			if !seen[pid] {
				seen[pid] = true
				matched = append(matched, pid)
			}
		}
	}

	pids := reclaimableChromePIDs(matched)
	if len(pids) == 0 {
		return 0
	}

	killed := forceKillPIDs(pids)
	slog.Info("shutdown: killed pinchtab chrome processes", "count", killed)
	return killed
}

func CleanupOrphanedChromeProcesses(profileDir string) {
	if profileDir == "" {
		return
	}
	killed := killOrphanedChromeByProfileDir(profileDir)
	if killed > 0 {
		slog.Info("cleanup: killed orphaned chrome processes", "count", killed, "profileDir", profileDir)
	}
}

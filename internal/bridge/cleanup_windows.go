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

// findPIDsByPowerShell finds Chrome PIDs whose command line contains the needle.
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

// taskkillPIDs force-kills processes and their children via taskkill /F /T.
func taskkillPIDs(pids []int) int {
	killed := 0
	for _, pid := range pids {
		cmd := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid))
		if cmd.Run() == nil {
			killed++
			slog.Info("cleanup: killed chrome process", "pid", pid)
		}
	}
	return killed
}

// killChromeByProfileDir finds and kills Chrome processes using the given profile directory.
func killChromeByProfileDir(profileDir string) int {
	pids := findPIDsByPowerShell(fmt.Sprintf("--user-data-dir=%s", profileDir))
	if len(pids) == 0 {
		return 0
	}
	return taskkillPIDs(pids)
}

// KillAllPinchtabChrome kills all Chrome processes spawned by PinchTab.
func KillAllPinchtabChrome() int {
	var pids []int
	seen := make(map[int]bool)

	for _, needle := range []string{"pinchtab-profile", ".pinchtab\\profiles"} {
		for _, pid := range findPIDsByPowerShell(needle) {
			if !seen[pid] {
				seen[pid] = true
				pids = append(pids, pid)
			}
		}
	}

	if len(pids) == 0 {
		return 0
	}

	killed := taskkillPIDs(pids)
	slog.Info("shutdown: killed pinchtab chrome processes", "count", killed)
	return killed
}

// CleanupOrphanedChromeProcesses kills orphaned Chrome processes on Windows.
func CleanupOrphanedChromeProcesses(profileDir string) {
	if profileDir == "" {
		return
	}
	killed := killChromeByProfileDir(profileDir)
	if killed > 0 {
		slog.Info("cleanup: killed orphaned chrome processes", "count", killed, "profileDir", profileDir)
	}
}

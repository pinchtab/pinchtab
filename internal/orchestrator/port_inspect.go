package orchestrator

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
)

func inspectPort(port string) PortInspection {
	portNum, err := parsePortNumber(port)
	if err != nil {
		return PortInspection{}
	}
	if isPortAvailable(port) {
		return PortInspection{Available: true}
	}

	pid, command, ok := findListeningProcess(portNum)
	if !ok {
		return PortInspection{}
	}
	return PortInspection{
		Available: false,
		PID:       pid,
		Command:   command,
	}
}

func findListeningProcess(port int) (int, string, bool) {
	if goruntime.GOOS == "linux" {
		if pid, command, ok := findListeningProcessLinux(port); ok {
			return pid, command, true
		}
	}
	return findListeningProcessLsof(port)
}

func findListeningProcessLinux(port int) (int, string, bool) {
	inodes := map[string]struct{}{}
	collectListeningSocketInodes(filepath.Join("/proc", "net", "tcp"), port, inodes)
	collectListeningSocketInodes(filepath.Join("/proc", "net", "tcp6"), port, inodes)
	if len(inodes) == 0 {
		return 0, "", false
	}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, "", false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		fdDir := filepath.Join("/proc", entry.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			target, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}
			inode, ok := socketInodeFromSymlink(target)
			if !ok {
				continue
			}
			if _, exists := inodes[inode]; !exists {
				continue
			}
			return pid, readLinuxProcessCommand(pid), true
		}
	}
	return 0, "", false
}

func collectListeningSocketInodes(path string, port int, inodes map[string]struct{}) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	first := true
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if first {
			first = false
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		if fields[3] != "0A" {
			continue
		}
		if !matchesProcNetPort(fields[1], port) {
			continue
		}
		inodes[fields[9]] = struct{}{}
	}
}

func matchesProcNetPort(localAddress string, port int) bool {
	parts := strings.Split(localAddress, ":")
	if len(parts) < 2 {
		return false
	}
	portHex := parts[len(parts)-1]
	value, err := strconv.ParseInt(portHex, 16, 32)
	if err != nil {
		return false
	}
	return int(value) == port
}

func socketInodeFromSymlink(target string) (string, bool) {
	if !strings.HasPrefix(target, "socket:[") || !strings.HasSuffix(target, "]") {
		return "", false
	}
	return strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]"), true
}

func readLinuxProcessCommand(pid int) string {
	cmdline, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err == nil {
		command := strings.TrimSpace(strings.ReplaceAll(string(cmdline), "\x00", " "))
		if command != "" {
			return command
		}
	}
	comm, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "comm"))
	if err == nil {
		return strings.TrimSpace(string(comm))
	}
	return ""
}

func findListeningProcessLsof(port int) (int, string, bool) {
	if _, err := exec.LookPath("lsof"); err != nil {
		return 0, "", false
	}

	cmd := exec.Command("lsof", "-nP", fmt.Sprintf("-iTCP:%d", port), "-sTCP:LISTEN", "-Fpc") // #nosec G204 -- static command, internal numeric port
	out, err := cmd.Output()
	if err != nil {
		return 0, "", false
	}

	var pid int
	var command string
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		switch line[0] {
		case 'p':
			value, err := strconv.Atoi(strings.TrimSpace(line[1:]))
			if err == nil {
				pid = value
			}
		case 'c':
			command = strings.TrimSpace(line[1:])
		}
		if pid > 0 {
			return pid, command, true
		}
	}
	return 0, "", false
}

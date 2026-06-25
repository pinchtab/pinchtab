package orchestrator

import (
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

func instanceIsActive(inst *InstanceInternal) bool {
	if inst == nil {
		return false
	}
	if inst.cmd != nil {
		return isProcessAlive(inst.cmd.PID())
	}
	return inst.Status == "starting" || inst.Status == "running" || inst.Status == "stopping"
}

func waitForProcessExit(pid int, timeout time.Duration) bool {
	if pid <= 0 {
		return true
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !isProcessAlive(pid) {
			return true
		}
		time.Sleep(150 * time.Millisecond)
	}
	return !isProcessAlive(pid)
}

var processAliveFunc = processAlive

func isProcessAlive(pid int) bool {
	return processAliveFunc(pid)
}

func isPortAvailable(port string) bool {
	portNum, err := parsePortNumber(port)
	if err != nil {
		return false
	}
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", portNum))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func tailLogLine(logs string) string {
	if logs == "" {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(logs), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		const max = 220
		if len(line) > max {
			return line[len(line)-max:]
		}
		return line
	}
	return ""
}

func mergeEnvWithOverrides(base []string, overrides map[string]string) []string {
	out := make([]string, 0, len(base)+len(overrides))
	for _, kv := range base {
		key, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if _, exists := overrides[key]; exists {
			continue
		}
		out = append(out, kv)
	}

	keys := make([]string, 0, len(overrides))
	for k := range overrides {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out = append(out, k+"="+overrides[k])
	}
	return out
}

func filterEnvWithPrefixes(env []string, prefixes ...string) []string {
	out := make([]string, 0, len(env))
	for _, e := range env {
		skip := false
		for _, p := range prefixes {
			if strings.HasPrefix(e, p) {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, e)
		}
	}
	return out
}

type ringBuffer struct {
	mu           sync.Mutex
	data         []byte
	max          int
	totalWritten uint64
}

func newRingBuffer(max int) *ringBuffer {
	return &ringBuffer{max: max, data: make([]byte, 0, max)}
}

func (rb *ringBuffer) Write(p []byte) (int, error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.data = append(rb.data, p...)
	if len(rb.data) > rb.max {
		rb.data = rb.data[len(rb.data)-rb.max:]
	}
	rb.totalWritten += uint64(len(p))
	return len(p), nil
}

func (rb *ringBuffer) String() string {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return string(rb.data)
}

// since returns log bytes written after the given absolute offset, the new
// absolute offset (totalWritten), and reset=true when offset fell behind the
// evicted window (caller must replace, not append). chunk is "" when nothing new.
func (rb *ringBuffer) since(offset uint64) (chunk string, newOffset uint64, reset bool) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	end := rb.totalWritten
	start := end - uint64(len(rb.data))
	if offset < start {
		return string(rb.data), end, true
	}
	if offset >= end {
		return "", end, false
	}
	return string(rb.data[offset-start:]), end, false
}

func tailChildLog(rb *ringBuffer, maxLines int) string {
	if rb == nil || maxLines <= 0 {
		return ""
	}
	raw := strings.TrimRight(rb.String(), "\n")
	if raw == "" {
		return ""
	}
	lines := strings.Split(raw, "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n")
}

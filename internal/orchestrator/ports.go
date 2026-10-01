package orchestrator

import (
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/pinchtab/pinchtab/internal/config"
)

type PortAllocator struct {
	mu            sync.Mutex
	start         int
	end           int
	allocated     map[int]bool
	nextCandidate int
	isAvailable   func(port int) bool
}

func NewPortAllocator(start, end int) *PortAllocator {
	if start < 1 || end < 1 || start > end {
		slog.Error("invalid port range", "start", start, "end", end)
		start = config.DefaultInstancePortStart
		end = config.DefaultInstancePortEnd
	}

	return &PortAllocator{
		start:         start,
		end:           end,
		allocated:     make(map[int]bool),
		nextCandidate: start,
		isAvailable:   isPortAvailableInt,
	}
}

func (pa *PortAllocator) AllocatePort() (int, error) {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	attempts := 0
	maxAttempts := pa.end - pa.start + 1

	for attempts < maxAttempts {
		candidate := pa.nextCandidate

		if candidate > pa.end {
			pa.nextCandidate = pa.start
			candidate = pa.start
		}

		pa.nextCandidate = candidate + 1

		if pa.allocated[candidate] {
			attempts++
			continue
		}

		if pa.isAvailable(candidate) {
			pa.allocated[candidate] = true
			slog.Debug("allocated port", "port", candidate)
			return candidate, nil
		}

		attempts++
	}

	return 0, fmt.Errorf("no available ports in range %d-%d", pa.start, pa.end)
}

func (pa *PortAllocator) ReservePort(port int) error {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	if port < pa.start || port > pa.end {
		return nil
	}
	if pa.allocated[port] {
		return fmt.Errorf("port %d already reserved", port)
	}
	pa.allocated[port] = true
	slog.Debug("reserved port", "port", port)
	return nil
}

func (pa *PortAllocator) ReleasePort(port int) {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	delete(pa.allocated, port)
	if port >= pa.start && port <= pa.end {
		pa.nextCandidate = port
	}
	slog.Debug("released port", "port", port)
}

func (pa *PortAllocator) IsAllocated(port int) bool {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	return pa.allocated[port]
}

func (pa *PortAllocator) AllocatedPorts() []int {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	ports := make([]int, 0, len(pa.allocated))
	for port := range pa.allocated {
		ports = append(ports, port)
	}
	return ports
}

func isPortAvailableInt(port int) bool {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}

func parsePortNumber(port string) (int, error) {
	value := strings.TrimSpace(port)
	if len(value) > 1 && value[0] == '0' {
		return 0, fmt.Errorf("invalid port %q: leading zeros not allowed", port)
	}
	portNum, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid port %q", port)
	}
	if portNum < 1 || portNum > 65535 {
		return 0, fmt.Errorf("port %d out of range", portNum)
	}
	return portNum, nil
}

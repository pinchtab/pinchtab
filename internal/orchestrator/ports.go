package orchestrator

import (
	"fmt"
	"log/slog"
	"net"
	"sync"
)

type PortAllocator struct {
	mu            sync.Mutex
	start         int
	end           int
	allocated     map[int]bool
	nextCandidate int
}

func NewPortAllocator(start, end int) *PortAllocator {
	if start < 1 || end < 1 || start > end {
		slog.Error("invalid port range", "start", start, "end", end)
		start = 9868
		end = 9968
	}

	return &PortAllocator{
		start:         start,
		end:           end,
		allocated:     make(map[int]bool),
		nextCandidate: start,
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

		if isPortAvailableInt(candidate) {
			pa.allocated[candidate] = true
			slog.Debug("allocated port", "port", candidate)
			return candidate, nil
		}

		attempts++
	}

	return 0, fmt.Errorf("no available ports in range %d-%d", pa.start, pa.end)
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

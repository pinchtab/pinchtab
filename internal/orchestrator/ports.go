package orchestrator

import (
	"fmt"
	"log/slog"
	"net"
	"sync"
)

// PortAllocator manages allocation of ports for instances within a configured range
type PortAllocator struct {
	mu            sync.Mutex
	start         int
	end           int
	allocated     map[int]bool // tracks which ports are allocated
	nextCandidate int
}

// NewPortAllocator creates a new port allocator for the given range
func NewPortAllocator(start, end int) *PortAllocator {
	if start < 1 || end < 1 || start > end {
		slog.Error("invalid port range", "start", start, "end", end)
		// Default to a safe range if invalid
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

// AllocatePort finds and allocates the next available port in the range
// Returns error if no ports are available
func (pa *PortAllocator) AllocatePort() (int, error) {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	attempts := 0
	maxAttempts := pa.end - pa.start + 1

	for attempts < maxAttempts {
		candidate := pa.nextCandidate

		// Wrap around to start if we've reached the end
		if candidate > pa.end {
			pa.nextCandidate = pa.start
			candidate = pa.start
		}

		pa.nextCandidate = candidate + 1

		// Skip if already allocated
		if pa.allocated[candidate] {
			attempts++
			continue
		}

		// Check if port is available (not in use by other processes)
		if isPortAvailableInt(candidate) {
			pa.allocated[candidate] = true
			slog.Debug("allocated port", "port", candidate)
			return candidate, nil
		}

		attempts++
	}

	return 0, fmt.Errorf("no available ports in range %d-%d", pa.start, pa.end)
}

// ReleasePort marks a port as no longer allocated
func (pa *PortAllocator) ReleasePort(port int) {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	delete(pa.allocated, port)
	slog.Debug("released port", "port", port)
}

// IsAllocated returns whether a port is currently allocated
func (pa *PortAllocator) IsAllocated(port int) bool {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	return pa.allocated[port]
}

// AllocatedPorts returns a copy of all allocated port numbers
func (pa *PortAllocator) AllocatedPorts() []int {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	ports := make([]int, 0, len(pa.allocated))
	for port := range pa.allocated {
		ports = append(ports, port)
	}
	return ports
}

// isPortAvailableInt checks if a port (as int) is available for binding
func isPortAvailableInt(port int) bool {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}

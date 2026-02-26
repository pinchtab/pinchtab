package orchestrator

import (
	"fmt"
	"net"
	"strconv"
)

// ValidatePort ensures the port is valid and safe to use
func ValidatePort(port string) error {
	if port == "" {
		return fmt.Errorf("port cannot be empty")
	}

	// Parse port number
	portNum, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("invalid port number: %s", port)
	}

	// Check valid port range - allow all valid TCP ports
	// Pinchtab instances can run on any available port
	if portNum < 1 || portNum > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}

	return nil
}

// ValidateHost ensures only localhost connections are allowed
func ValidateHost(host string) error {
	// Only allow localhost connections
	allowedHosts := []string{
		"localhost",
		"127.0.0.1",
		"[::1]",
		"::1",
	}

	for _, allowed := range allowedHosts {
		if host == allowed {
			return nil
		}
	}

	// Also check if it's a loopback IP
	ip := net.ParseIP(host)
	if ip != nil && ip.IsLoopback() {
		return nil
	}

	return fmt.Errorf("only localhost connections are allowed")
}

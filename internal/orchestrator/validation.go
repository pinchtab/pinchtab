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

	// Check valid port range
	if portNum < 1024 || portNum > 65535 {
		return fmt.Errorf("port must be between 1024 and 65535")
	}

	// Disallow common sensitive ports
	blockedPorts := []int{
		3306,  // MySQL
		5432,  // PostgreSQL
		6379,  // Redis
		9200,  // Elasticsearch
		27017, // MongoDB
		11211, // Memcached
		2049,  // NFS
		111,   // RPC
		22,    // SSH (if somehow in high range)
		25,    // SMTP
		110,   // POP3
		143,   // IMAP
	}

	for _, blocked := range blockedPorts {
		if portNum == blocked {
			return fmt.Errorf("port %d is not allowed", portNum)
		}
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

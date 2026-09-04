//go:build !windows

package server

import (
	"errors"
	"syscall"
)

// isAddrInUse reports whether err is the kernel refusing a bind because
// something already holds the address.
func isAddrInUse(err error) bool {
	return errors.Is(err, syscall.EADDRINUSE)
}

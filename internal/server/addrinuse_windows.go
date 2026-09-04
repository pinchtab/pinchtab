//go:build windows

package server

import (
	"errors"
	"syscall"
)

// wsaAddrInUse is winsock's WSAEADDRINUSE, which is what Windows actually
// returns for a taken address.
//
// It is not syscall.EADDRINUSE. Go's Windows errno table defines that name as a
// placeholder in the APPLICATION_ERROR range — 536870914 — which no syscall
// ever returns, so errors.Is(err, syscall.EADDRINUSE) is false for every real
// bind conflict on this platform. A plain check against it therefore compiled,
// ran, and silently answered "no" every time.
//
// Declared inline for the same reason internal/bridge/state_lock_windows.go
// declares its ERROR_* codes: it keeps golang.org/x/sys/windows out of the
// import graph for the sake of one constant.
const wsaAddrInUse syscall.Errno = 10048

func isAddrInUse(err error) bool {
	if errors.Is(err, syscall.EADDRINUSE) {
		return true
	}
	var errno syscall.Errno
	return errors.As(err, &errno) && errno == wsaAddrInUse
}

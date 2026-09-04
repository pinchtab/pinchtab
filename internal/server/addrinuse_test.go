package server

import (
	"errors"
	"fmt"
	"net"
	"syscall"
	"testing"
)

// TestIsAddrInUseRecognizesARealBindConflict pins the platform's own answer
// rather than a constant this test picked.
//
// The check used to be errors.Is(err, syscall.EADDRINUSE) inline. That is
// correct on unix and always false on Windows, where a taken address comes back
// as winsock's WSAEADDRINUSE (10048) and syscall.EADDRINUSE names an
// APPLICATION_ERROR placeholder (536870914) that nothing returns. The call
// compiled and ran on both platforms and answered "no" on one of them, so
// `pinchtab server` printed the raw winsock sentence with none of the guidance
// that explains it — on what is the most ordinary startup failure there is,
// starting a server while the daemon already holds the port.
func TestIsAddrInUseRecognizesARealBindConflict(t *testing.T) {
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not take a port to hold: %v", err)
	}
	defer func() { _ = held.Close() }()

	_, err = net.Listen("tcp", held.Addr().String())
	if err == nil {
		t.Fatal("second listen on a held address succeeded; no conflict to classify")
	}
	if !isAddrInUse(err) {
		var errno syscall.Errno
		errors.As(err, &errno)
		t.Fatalf("isAddrInUse did not recognize a real bind conflict: %v (errno %d)", err, uintptr(errno))
	}
}

// Unrelated failures must not collect the port-in-use advice, which would send
// the reader looking for a process that is not there.
func TestIsAddrInUseIgnoresOtherErrors(t *testing.T) {
	for _, err := range []error{
		fmt.Errorf("some other failure"),
		syscall.ENOENT,
		fmt.Errorf("wrapped: %w", syscall.EACCES),
	} {
		if isAddrInUse(err) {
			t.Errorf("isAddrInUse(%v) = true, want false", err)
		}
	}
}

// TestStartupFatalHintsFireForARealBindConflict is the positive half that was
// missing. TestStartupFatalHintsOnlyForAddressInUse already pins that an
// unrelated error produces nothing — which stayed true on Windows for the worst
// possible reason, because the classifier said "not in use" to everything.
func TestStartupFatalHintsFireForARealBindConflict(t *testing.T) {
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not take a port to hold: %v", err)
	}
	defer func() { _ = held.Close() }()
	_, bindErr := net.Listen("tcp", held.Addr().String())
	if bindErr == nil {
		t.Fatal("second listen on a held address succeeded")
	}

	if got := startupFatalHints(bindErr); len(got) == 0 {
		t.Error("a real bind conflict produced no hints")
	}
}

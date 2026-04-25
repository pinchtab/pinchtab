package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const MaxToolOutputChars = 2400

type PersistentShell struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser

	mu      sync.Mutex
	closed  bool
	workDir string
	outChan chan string
	errChan chan error
}

func NewPersistentShell(workDir string) (*PersistentShell, error) {
	cmd := exec.Command("/bin/bash", "--norc", "--noprofile")
	cmd.Dir = workDir

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	// Redirect stderr to stdout so we capture everything
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, fmt.Errorf("start shell: %w", err)
	}

	ps := &PersistentShell{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		workDir: workDir,
		outChan: make(chan string, 100),
		errChan: make(chan error, 1),
	}

	go ps.readLoop(stdout, ps.outChan, ps.errChan)

	// Wait for shell to be ready
	time.Sleep(100 * time.Millisecond)

	return ps, nil
}

func (ps *PersistentShell) readLoop(stdout io.Reader, outChan chan<- string, errChan chan<- error) {
	buf := make([]byte, 4096)
	for {
		n, err := stdout.Read(buf)
		if n > 0 {
			outChan <- string(buf[:n])
		}
		if err != nil {
			errChan <- err
			return
		}
	}
}

func (ps *PersistentShell) Run(command string, timeout time.Duration) (output string, exitCode int, err error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if ps.closed {
		if err := ps.resetLocked(); err != nil {
			return "", -1, fmt.Errorf("shell reset failed: %w", err)
		}
	}

	nonce := randomHex(8)
	marker := fmt.Sprintf("__DONE_%s__", nonce)

	// Strip any cd prefix the model might have added, then cd to workDir
	cleanCmd := command
	if idx := strings.Index(command, " && "); idx > 0 && strings.HasPrefix(command, "cd ") {
		cleanCmd = command[idx+4:]
	}
	wrapped := fmt.Sprintf("cd %q && %s; echo \"%s:$?\"\n", ps.workDir, cleanCmd, marker)
	if _, err := io.WriteString(ps.stdin, wrapped); err != nil {
		ps.closed = true
		return "", -1, fmt.Errorf("write command: %w", err)
	}

	// Collect output until we see the marker
	var buffer bytes.Buffer
	deadline := time.After(timeout)
	markerPrefix := marker + ":"

	for {
		select {
		case chunk := <-ps.outChan:
			buffer.WriteString(chunk)
			content := buffer.String()

			// Look for the marker
			if idx := strings.Index(content, markerPrefix); idx >= 0 {
				// Found marker, extract output and exit code
				output := content[:idx]
				rest := content[idx+len(markerPrefix):]

				// Parse exit code (first number after marker)
				var code int
				_, _ = fmt.Sscanf(rest, "%d", &code)

				// Clean up output
				output = strings.TrimPrefix(output, "\n")
				output = strings.TrimSuffix(output, "\n")

				return output, code, nil
			}

		case err := <-ps.errChan:
			ps.closed = true
			return "", -1, fmt.Errorf("shell closed: %w", err)

		case <-deadline:
			ps.closeLocked(true)
			return "", -1, fmt.Errorf("command timed out after %v: %s", timeout, command)
		}
	}
}

func (ps *PersistentShell) closeLocked(force bool) {
	if ps.closed {
		return
	}
	ps.closed = true

	if force {
		_ = ps.cmd.Process.Kill()
	} else {
		_, _ = io.WriteString(ps.stdin, "exit\n")
	}
	_ = ps.stdin.Close()
	_ = ps.cmd.Wait()
}

func (ps *PersistentShell) Close(force bool) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.closeLocked(force)
}

func (ps *PersistentShell) resetLocked() error {
	if !ps.closed {
		ps.closeLocked(true)
	}

	cmd := exec.Command("/bin/bash", "--norc", "--noprofile")
	cmd.Dir = ps.workDir

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("stdout pipe: %w", err)
	}

	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return fmt.Errorf("start shell: %w", err)
	}

	ps.cmd = cmd
	ps.stdin = stdin
	ps.stdout = stdout
	ps.closed = false
	ps.outChan = make(chan string, 100)
	ps.errChan = make(chan error, 1)

	go ps.readLoop(stdout, ps.outChan, ps.errChan)
	time.Sleep(100 * time.Millisecond)

	return nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func TrimToolOutput(text string) string {
	if len(text) <= MaxToolOutputChars {
		return text
	}
	return fmt.Sprintf("%s\n\n[output truncated: %d more chars]", text[:MaxToolOutputChars], len(text)-MaxToolOutputChars)
}

func FormatToolResult(command string, exitCode int, output string) string {
	return fmt.Sprintf("$ %s\n[exit_code=%d]\n%s", command, exitCode, TrimToolOutput(output))
}

package main

import (
	"strings"
	"testing"
	"time"
)

func TestShellEcho(t *testing.T) {
	ps, err := NewPersistentShell(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer ps.Close(false)

	out, code, err := ps.Run("echo hello", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Errorf("exit code = %d; want 0", code)
	}
	if strings.TrimSpace(out) != "hello" {
		t.Errorf("output = %q; want 'hello'", out)
	}
}

func TestShellNonZeroExit(t *testing.T) {
	ps, err := NewPersistentShell(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer ps.Close(false)

	out, code, err := ps.Run("(exit 42)", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if code != 42 {
		t.Errorf("exit code = %d; want 42", code)
	}
	if out != "" {
		t.Errorf("output = %q; want empty", out)
	}
}

func TestShellTimeout(t *testing.T) {
	ps, err := NewPersistentShell(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer ps.Close(true)

	_, _, err = ps.Run("sleep 60", 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("error = %v; want timeout", err)
	}
}

func TestShellEnvPersistence(t *testing.T) {
	ps, err := NewPersistentShell(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer ps.Close(false)

	_, code1, err := ps.Run("export TESTVAR=foobar123", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if code1 != 0 {
		t.Fatalf("export exit code = %d", code1)
	}

	out, code2, err := ps.Run("echo $TESTVAR", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if code2 != 0 {
		t.Errorf("echo exit code = %d", code2)
	}
	if strings.TrimSpace(out) != "foobar123" {
		t.Errorf("output = %q; want 'foobar123'", out)
	}
}

func TestShellLargeOutput(t *testing.T) {
	ps, err := NewPersistentShell(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer ps.Close(false)

	out, code, err := ps.Run("seq 1 50000", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Errorf("exit code = %d; want 0", code)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 50000 {
		t.Errorf("got %d lines; want 50000", len(lines))
	}
	if lines[0] != "1" || lines[49999] != "50000" {
		t.Errorf("first/last lines incorrect: %q / %q", lines[0], lines[49999])
	}
}

func TestTrimToolOutput(t *testing.T) {
	short := "hello world"
	if got := TrimToolOutput(short); got != short {
		t.Errorf("short text modified: %q", got)
	}

	long := strings.Repeat("x", 3000)
	trimmed := TrimToolOutput(long)
	if len(trimmed) > MaxToolOutputChars+100 {
		t.Errorf("trimmed too long: %d chars", len(trimmed))
	}
	if !strings.Contains(trimmed, "[output truncated:") {
		t.Error("missing truncation marker")
	}
}

func TestFormatToolResult(t *testing.T) {
	result := FormatToolResult("ls -la", 0, "file.txt")
	if !strings.HasPrefix(result, "$ ls -la") {
		t.Errorf("missing command prefix: %q", result)
	}
	if !strings.Contains(result, "[exit_code=0]") {
		t.Errorf("missing exit code: %q", result)
	}
}

func TestShellAutoReset(t *testing.T) {
	ps, err := NewPersistentShell(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer ps.Close(true)

	out1, code1, err := ps.Run("echo before", 5*time.Second)
	if err != nil {
		t.Fatalf("first command failed: %v", err)
	}
	if code1 != 0 || strings.TrimSpace(out1) != "before" {
		t.Errorf("first command: code=%d output=%q", code1, out1)
	}

	ps.Close(true)

	out2, code2, err := ps.Run("echo after", 5*time.Second)
	if err != nil {
		t.Fatalf("second command failed after reset: %v", err)
	}
	if code2 != 0 || strings.TrimSpace(out2) != "after" {
		t.Errorf("second command after reset: code=%d output=%q", code2, out2)
	}
}

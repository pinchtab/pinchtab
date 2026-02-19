package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

type Orchestrator struct {
	instances map[string]*Instance
	baseDir   string
	binary    string
	mu        sync.RWMutex
	client    *http.Client
}

type Instance struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Profile   string    `json:"profile"`
	Port      string    `json:"port"`
	Headless  bool      `json:"headless"`
	Status    string    `json:"status"`
	PID       int       `json:"pid,omitempty"`
	StartedAt time.Time `json:"startedAt"`
	Error     string    `json:"error,omitempty"`
	TabCount  int       `json:"tabCount"`
	URL       string    `json:"url"`

	cmd    *exec.Cmd
	cancel context.CancelFunc
	logBuf *ringBuffer
}

type ringBuffer struct {
	mu   sync.Mutex
	data []byte
	max  int
}

func newRingBuffer(max int) *ringBuffer {
	return &ringBuffer{max: max, data: make([]byte, 0, max)}
}

func (rb *ringBuffer) Write(p []byte) (int, error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.data = append(rb.data, p...)
	if len(rb.data) > rb.max {
		rb.data = rb.data[len(rb.data)-rb.max:]
	}
	return len(p), nil
}

func (rb *ringBuffer) String() string {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return string(rb.data)
}

func NewOrchestrator(baseDir string) *Orchestrator {
	binDir := filepath.Join(filepath.Dir(baseDir), "bin")
	stableBin := filepath.Join(binDir, "pinchtab")
	exe, _ := os.Executable()
	binary := exe
	if binary == "" {
		binary = os.Args[0]
	}

	if err := os.MkdirAll(binDir, 0755); err != nil {
		slog.Warn("failed to create bin directory", "path", binDir, "err", err)
	}

	if exe != "" {
		if err := installStableBinary(exe, stableBin); err != nil {
			slog.Warn("failed to install pinchtab binary", "path", stableBin, "err", err)
		} else {
			slog.Info("installed pinchtab binary", "path", stableBin)
		}
	}

	if _, err := os.Stat(binary); err != nil {
		if _, stableErr := os.Stat(stableBin); stableErr == nil {
			binary = stableBin
		}
	}

	return &Orchestrator{
		instances: make(map[string]*Instance),
		baseDir:   baseDir,
		binary:    binary,
		client:    &http.Client{Timeout: 3 * time.Second},
	}
}

func installStableBinary(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, in)
	return err
}

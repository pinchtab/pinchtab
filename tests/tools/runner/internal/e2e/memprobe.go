package e2e

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type memoryResult struct {
	containers []containerMemory
	note       string
}

type memoryProbe struct {
	stop   chan struct{}
	result chan memoryResult
}

const (
	dockerProbeTimeout    = 10 * time.Second
	memorySamplerDeadline = 15 * time.Second
)

func (r *Runner) startMemorySampler(composeFile string) *memoryProbe {
	probe := &memoryProbe{stop: make(chan struct{}), result: make(chan memoryResult, 1)}
	go func() {
		acc := newMemoryAccumulator()
		firstErr := ""
		note := func(err error) {
			if err != nil && firstErr == "" {
				firstErr = err.Error()
			}
		}

		stackIDs, err := r.stackContainerIDs(composeFile)
		note(err)
		ids := make([]string, 0, len(stackIDs))
		for id := range stackIDs {
			ids = append(ids, id)
		}

		sampleOnce := func() {
			if len(ids) == 0 {
				return
			}
			out, err := r.dockerStatsSnapshot(ids)
			if err != nil {
				note(err)
				return
			}
			for _, s := range selectStackSamples(out, stackIDs) {
				acc.add(s)
			}
		}

		sampleOnce()
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-probe.stop:
				sampleOnce()
				containers := acc.reduce()
				reason := ""
				if len(containers) == 0 {
					reason = "no pinchtab container memory sampled"
					if firstErr != "" {
						reason += " (" + firstErr + ")"
					}
				}
				probe.result <- memoryResult{containers: containers, note: reason}
				return
			case <-ticker.C:
				sampleOnce()
			}
		}
	}()
	return probe
}

func (p *memoryProbe) finish() memoryResult {
	close(p.stop)
	select {
	case result := <-p.result:
		return result
	case <-time.After(memorySamplerDeadline):
		return memoryResult{note: "memory sampler did not finish within " + memorySamplerDeadline.String()}
	}
}

func (r *Runner) applyMemoryResult(result memoryResult) {
	if len(result.containers) == 0 {
		r.mem = nil
		if result.note != "" {
			_, _ = fmt.Fprintf(r.stdout, "  memory: %s\n", result.note)
		}
		return
	}
	r.mem = &suiteMemory{Provider: r.provider(), Containers: result.containers}
}

func (r *Runner) stackContainerIDs(composeFile string) (map[string]bool, error) {
	out, err := r.captureWithTimeout(r.composeArgs(composeFile, "ps", "-q"))
	if err != nil {
		return nil, fmt.Errorf("docker compose ps: %w", err)
	}
	ids := map[string]bool{}
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		if id := strings.TrimSpace(line); id != "" {
			ids[id] = true
		}
	}
	return ids, nil
}

func (r *Runner) dockerStatsSnapshot(ids []string) (string, error) {
	command := append([]string{"docker", "stats", "--no-stream", "--format", "{{.ID}},{{.Name}},{{.MemUsage}},{{.PIDs}}"}, ids...)
	out, err := r.captureWithTimeout(command)
	if err != nil {
		return "", fmt.Errorf("docker stats unavailable: %w", err)
	}
	return out, nil
}

func (r *Runner) captureWithTimeout(command []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dockerProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, command[0], command[1:]...) // #nosec G204 -- commands are fixed docker/compose invocations
	cmd.Dir = r.repoRoot
	cmd.Env = os.Environ()
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("timed out after %s", dockerProbeTimeout)
	}
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(buf.String()))
	}
	return buf.String(), nil
}

func isPinchtabBrowserContainer(name string) bool {
	return strings.Contains(name, "pinchtab") &&
		!strings.Contains(name, "fixtures") &&
		!strings.Contains(name, "runner")
}

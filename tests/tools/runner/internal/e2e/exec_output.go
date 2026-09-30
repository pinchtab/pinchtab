package e2e

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (r *Runner) printSuiteStart(def suiteDef) {
	_, _ = fmt.Fprintf(r.stdout, "== %s ==\n", def.Title)
	if r.args.Filter != "" {
		_, _ = fmt.Fprintf(r.stdout, "  filter: %s\n", r.args.Filter)
	} else {
		_, _ = fmt.Fprintln(r.stdout, "  filter: none")
	}
	if r.args.Test != "" {
		_, _ = fmt.Fprintf(r.stdout, "  test:   %s\n", r.args.Test)
	}
	if r.args.Extra != "" {
		_, _ = fmt.Fprintf(r.stdout, "  extra:  %s\n", r.args.Extra)
	}
	_, _ = fmt.Fprintf(r.stdout, "  logs:   %s\n\n", r.logsMode)
}

func (r *Runner) runLoggedCommand(label, outputFile string, command []string) int {
	if r.args.DryRun {
		_, _ = fmt.Fprintf(r.stdout, "# %s\n%s\n", label, shellQuoteArgs(command))
		return 0
	}

	if r.logsMode == "show" {
		_, _ = fmt.Fprintf(r.stdout, "%s\n", label)
		return r.runStreamingCommand(command, outputFile)
	}
	if outputFile == "" {
		outputFile = stackOutput
	}
	return r.runCompactCommand(label, outputFile, command)
}

func (r *Runner) appendSuiteResult(def suiteDef, status string, duration time.Duration, name string) (err error) {
	if r.args.DryRun {
		return nil
	}
	outputPath := filepath.Join(r.repoRoot, def.Output)
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(outputPath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
	}()
	if stat, err := file.Stat(); err == nil && stat.Size() > 0 {
		var last [1]byte
		if _, err := file.ReadAt(last[:], stat.Size()-1); err == nil && last[0] != '\n' {
			if _, err := fmt.Fprintln(file); err != nil {
				return err
			}
		}
	}
	_, err = fmt.Fprintf(file, "E2E_RESULT\t%s\t%d\t%s\n", status, duration.Milliseconds(), name)
	return
}

func (r *Runner) runStreamingCommand(command []string, outputFile string) int {
	cmd := exec.Command(command[0], command[1:]...) // #nosec G204 -- commands are constructed from fixed compose/script inputs.
	cmd.Dir = r.repoRoot
	var logFile *os.File
	var stdoutFilter *structuredEventTee
	if outputFile != "" {
		outputPath := filepath.Join(r.repoRoot, outputFile)
		if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
			_, _ = fmt.Fprintf(r.stderr, "e2e: failed to prepare output path: %v\n", err)
			return 1
		}
		file, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			_, _ = fmt.Fprintf(r.stderr, "e2e: failed to open %s: %v\n", outputFile, err)
			return 1
		}
		logFile = file
		defer func() {
			if closeErr := logFile.Close(); closeErr != nil {
				_, _ = fmt.Fprintf(r.stderr, "e2e: failed to close %s: %v\n", outputFile, closeErr)
			}
		}()
		stdoutFilter = &structuredEventTee{human: r.stdout, log: logFile}
		cmd.Stdout = stdoutFilter
		cmd.Stderr = io.MultiWriter(r.stderr, logFile)
	} else {
		cmd.Stdout = r.stdout
		cmd.Stderr = r.stderr
	}
	cmd.Stdin = os.Stdin
	cmd.Env = os.Environ()
	err := cmd.Run()
	if stdoutFilter != nil {
		if flushErr := stdoutFilter.Flush(); flushErr != nil {
			_, _ = fmt.Fprintf(r.stderr, "e2e: failed to write output: %v\n", flushErr)
			return 1
		}
	}
	if err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			return exitErr.ExitCode()
		}
		_, _ = fmt.Fprintf(r.stderr, "e2e: failed to run %s: %v\n", shellQuoteArgs(command), err)
		return 1
	}
	return 0
}

type structuredEventTee struct {
	human   io.Writer
	log     io.Writer
	pending []byte
}

func (w *structuredEventTee) Write(p []byte) (int, error) {
	if _, err := w.log.Write(p); err != nil {
		return 0, err
	}

	remaining := p
	for len(remaining) > 0 {
		idx := bytes.IndexByte(remaining, '\n')
		if idx < 0 {
			w.pending = append(w.pending, remaining...)
			break
		}
		line := append(w.pending, remaining[:idx+1]...)
		w.pending = w.pending[:0]
		if err := w.writeHumanLine(line); err != nil {
			return 0, err
		}
		remaining = remaining[idx+1:]
	}
	return len(p), nil
}

func (w *structuredEventTee) Flush() error {
	if len(w.pending) == 0 {
		return nil
	}
	line := w.pending
	w.pending = nil
	return w.writeHumanLine(line)
}

func (w *structuredEventTee) writeHumanLine(line []byte) error {
	text := strings.TrimRight(string(line), "\r\n")
	if strings.HasPrefix(text, "E2E_RESULT\t") || strings.HasPrefix(text, "E2E_RESULT_SUMMARY\t") {
		return nil
	}
	_, err := w.human.Write(line)
	return err
}

func (r *Runner) runCompactCommand(label, outputFile string, command []string) int {
	outputPath := filepath.Join(r.repoRoot, outputFile)
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		_, _ = fmt.Fprintf(r.stderr, "e2e: failed to prepare output path: %v\n", err)
		return 1
	}
	file, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		_, _ = fmt.Fprintf(r.stderr, "e2e: failed to open %s: %v\n", outputFile, err)
		return 1
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			_, _ = fmt.Fprintf(r.stderr, "e2e: failed to close %s: %v\n", outputFile, closeErr)
		}
	}()

	prog := newProgressLine(r.stdout)
	prog.Update(fmt.Sprintf("  %s...", label))

	cmd := exec.Command(command[0], command[1:]...) // #nosec G204 -- commands are constructed from fixed compose/script inputs.
	cmd.Dir = r.repoRoot
	cmd.Stdout = file
	cmd.Stderr = file
	cmd.Stdin = os.Stdin
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		prog.Clear()
		_, _ = fmt.Fprintf(r.stderr, "e2e: failed to start %s: %v\n", shellQuoteArgs(command), err)
		return 1
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	lastRunning := ""
	passed := 0
	failed := 0
	for {
		select {
		case err := <-done:
			ticker.Stop()
			passed, failed = r.countResults(outputFile)
			if err == nil {
				if passed+failed > 0 {
					prog.Complete(fmt.Sprintf("  %s: %d passed", label, passed))
				} else {
					prog.Complete(fmt.Sprintf("  %s: done", label))
				}
				return 0
			}
			if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
				if passed+failed > 0 {
					prog.Complete(fmt.Sprintf("  %s: %d passed, %d failed", label, passed, failed))
				} else {
					prog.Complete(fmt.Sprintf("  %s: failed (exit %d)", label, exitErr.ExitCode()))
				}
				return exitErr.ExitCode()
			}
			prog.Clear()
			_, _ = fmt.Fprintf(r.stderr, "e2e: failed while running %s: %v\n", shellQuoteArgs(command), err)
			return 1
		case <-ticker.C:
			p, f := r.countResults(outputFile)
			passed, failed = p, f
			name := r.readLastRunningName(outputFile)
			if name != "" {
				name = strings.TrimSuffix(name, ".sh")
			}
			if name != "" && name != lastRunning {
				lastRunning = name
			}
			total := passed + failed
			if total > 0 && lastRunning != "" {
				prog.Update(fmt.Sprintf("  %s [%d done] %s", label, total, lastRunning))
			} else if lastRunning != "" {
				prog.Update(fmt.Sprintf("  %s: %s", label, lastRunning))
			}
		}
	}
}

func (r *Runner) countResults(outputFile string) (passed, failed int) {
	file, err := os.Open(filepath.Join(r.repoRoot, outputFile))
	if err != nil {
		return 0, 0
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "E2E_RESULT\tpassed\t") {
			passed++
		} else if strings.HasPrefix(line, "E2E_RESULT\tfailed\t") {
			failed++
		}
	}
	return passed, failed
}

func (r *Runner) readLastRunningName(outputFile string) string {
	file, err := os.Open(filepath.Join(r.repoRoot, outputFile))
	if err != nil {
		return ""
	}
	defer func() {
		_ = file.Close()
	}()

	scanner := bufio.NewScanner(file)
	last := ""
	for scanner.Scan() {
		line := scanner.Text()
		if _, after, found := strings.Cut(line, "Running:"); found {
			name := strings.TrimSpace(after)
			name = strings.TrimSuffix(name, "\x1b[0m")
			if name != "" {
				last = name
			}
		}
	}
	return last
}

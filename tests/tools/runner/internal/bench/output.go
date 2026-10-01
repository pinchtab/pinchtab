package bench

import (
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	colorReset  = "\033[0m"
	colorDim    = "\033[2m"
	colorBold   = "\033[1m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	clearLine   = "\033[2K\r"
)

type OutputWriter struct {
	w       io.Writer
	verbose bool
}

func NewOutputWriter(w io.Writer, verbose bool) *OutputWriter {
	return &OutputWriter{w: w, verbose: verbose}
}

func (o *OutputWriter) Turn(turn, maxTurns int) {
	if !o.verbose {
		return
	}
	_, _ = fmt.Fprintf(o.w, "\n%s─── Turn %d/%d ───%s\n", colorDim, turn, maxTurns, colorReset)
}

type Spinner struct {
	w         io.Writer
	stop      chan struct{}
	done      chan struct{}
	message   string
	startTime time.Time
	tokens    int
}

func (o *OutputWriter) StartSpinner(message string) *Spinner {
	if !o.verbose {
		return nil
	}
	s := &Spinner{
		w:         o.w,
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
		message:   message,
		startTime: time.Now(),
	}
	go s.run()
	return s
}

func (s *Spinner) UpdateTokens(tokens int) {
	if s != nil {
		s.tokens = tokens
	}
}

func (s *Spinner) run() {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	i := 0
	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()
	defer close(s.done)

	for {
		select {
		case <-s.stop:
			_, _ = fmt.Fprint(s.w, clearLine)
			return
		case <-ticker.C:
			elapsed := time.Since(s.startTime).Round(time.Second)
			tokenStr := ""
			if s.tokens > 0 {
				tokenStr = fmt.Sprintf(" · ↓ %d tokens", s.tokens)
			}
			_, _ = fmt.Fprintf(s.w, "%s%s✶ %s%s (%v%s)%s",
				clearLine, colorCyan, colorReset, s.message, elapsed, tokenStr, colorReset)
			i = (i + 1) % len(frames)
		}
	}
}

func (s *Spinner) Stop() {
	if s == nil {
		return
	}
	close(s.stop)
	<-s.done
}

func (o *OutputWriter) ToolCall(cmd string) {
	if !o.verbose {
		return
	}
	display := cleanupCommand(cmd)
	if len(display) > 80 {
		display = display[:77] + "..."
	}
	_, _ = fmt.Fprintf(o.w, "%s▶ %s%s\n", colorCyan, display, colorReset)
}

func cleanupCommand(cmd string) string {
	// Strip "cd /path/to/benchmark && " prefix
	if idx := strings.Index(cmd, " && "); idx > 0 && strings.HasPrefix(cmd, "cd ") {
		return cmd[idx+4:]
	}
	return cmd
}

func (o *OutputWriter) ToolResult(cmd string, exitCode int, output string, duration time.Duration) {
	if !o.verbose {
		return
	}
	status := fmt.Sprintf("%s✓%s", colorGreen, colorReset)
	if exitCode != 0 {
		status = fmt.Sprintf("%s✗ exit %d%s", colorYellow, exitCode, colorReset)
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	var preview strings.Builder
	maxPreviewLines := 2
	if len(lines) > 0 {
		for i := 0; i < len(lines) && i < maxPreviewLines; i++ {
			line := lines[i]
			if len(line) > 70 {
				line = line[:67] + "..."
			}
			if i > 0 {
				preview.WriteString("\n       ")
			}
			preview.WriteString(line)
		}
		if len(lines) > maxPreviewLines {
			fmt.Fprintf(&preview, " %s(+%d lines)%s", colorDim, len(lines)-maxPreviewLines, colorReset)
		}
	}

	_, _ = fmt.Fprintf(o.w, "  %s %s %s[%v]%s\n", status, preview.String(), colorDim, duration.Round(time.Millisecond), colorReset)
}

func (o *OutputWriter) Progress(answered, verified, failed int) {
	if !o.verbose {
		return
	}
	_, _ = fmt.Fprintf(o.w, "%s  ℹ Progress: %d answered, %d verified, %d failed%s\n",
		colorDim, answered, verified, failed, colorReset)
}

func (o *OutputWriter) StepRecorded(stepID string) {
	if !o.verbose {
		return
	}
	_, _ = fmt.Fprintf(o.w, "%s✓ Step %s recorded%s\n", colorGreen, stepID, colorReset)
}

func (o *OutputWriter) APICall(provider, model string, inputTokens, outputTokens, cacheCreateTokens, cacheReadTokens int) {
	if !o.verbose {
		return
	}
	// in = uncached input; cc = cache_create; cr = cache_read; out = output
	_, _ = fmt.Fprintf(o.w, "%s  ← %s/%s: %d in, %d cc, %d cr, %d out%s\n",
		colorDim, provider, model, inputTokens, cacheCreateTokens, cacheReadTokens, outputTokens, colorReset)
}

func (o *OutputWriter) FinalText(text string) {
	if text != "" {
		_, _ = fmt.Fprintln(o.w, text)
	}
}

func (o *OutputWriter) Summary(usage UsageCounters, provider string) {
	_, _ = fmt.Fprintf(o.w, "\n%s[run-usage]%s provider=%s requests=%d input=%d cache_create=%d cache_read=%d output=%d total=%d\n",
		colorDim, colorReset,
		provider,
		usage.RequestCount,
		usage.InputTokens,
		usage.CacheCreationInputTokens,
		usage.CacheReadInputTokens,
		usage.OutputTokens,
		usage.InputTokens+usage.CacheCreationInputTokens+usage.CacheReadInputTokens+usage.OutputTokens,
	)
}

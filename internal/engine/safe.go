package engine

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/pinchtab/pinchtab/internal/idpi"
)

// SafeEngine wraps any Engine with IDPI pre/post security checks.
// This ensures all engines (lite, chrome, mix) go through the same
// security pipeline.
type SafeEngine struct {
	inner Engine
	guard idpi.Guard
	wrap  bool // whether to wrap content with trust-boundary markers
}

// NewSafeEngine creates a SafeEngine decorator around the given engine.
// If guard is nil or not enabled, the inner engine is returned unwrapped.
func NewSafeEngine(inner Engine, guard idpi.Guard, wrapContent bool) Engine {
	if guard == nil || !guard.Enabled() {
		return inner
	}
	return &SafeEngine{
		inner: inner,
		guard: guard,
		wrap:  wrapContent,
	}
}

func (s *SafeEngine) Name() string               { return s.inner.Name() }
func (s *SafeEngine) Capabilities() []Capability { return s.inner.Capabilities() }
func (s *SafeEngine) Close() error               { return s.inner.Close() }

func (s *SafeEngine) Navigate(ctx context.Context, url string) (*NavigateResult, error) {
	// Pre-flight: IDPI domain check.
	domainResult := s.guard.CheckDomain(url)
	if domainResult.Blocked {
		return nil, fmt.Errorf("navigation blocked by IDPI: %s", domainResult.Reason)
	}

	result, err := s.inner.Navigate(ctx, url)
	if err != nil {
		return nil, err
	}

	if domainResult.Threat {
		slog.Warn("IDPI domain warning on navigate", "url", url, "reason", domainResult.Reason)
	}

	return result, nil
}

func (s *SafeEngine) Snapshot(ctx context.Context, tabID, filter string) (*SnapshotResult, error) {
	result, err := s.inner.Snapshot(ctx, tabID, filter)
	if err != nil {
		return nil, err
	}

	// Post-flight: scan node names and values for injection patterns.
	var sb strings.Builder
	for _, n := range result.Nodes {
		if n.Name != "" || n.Value != "" {
			sb.WriteString(n.Name)
			if n.Name != "" && n.Value != "" {
				sb.WriteByte(' ')
			}
			sb.WriteString(n.Value)
			sb.WriteByte('\n')
		}
	}

	scanResult := s.guard.ScanContent(sb.String())
	if scanResult.Blocked {
		return nil, fmt.Errorf("snapshot blocked by IDPI scanner: %s", scanResult.Reason)
	}
	if scanResult.Threat {
		result.IDPIWarning = scanResult.Reason
		slog.Warn("IDPI content warning on snapshot", "engine", result.Engine, "reason", scanResult.Reason)
	}

	return result, nil
}

func (s *SafeEngine) Text(ctx context.Context, tabID string) (*TextResult, error) {
	result, err := s.inner.Text(ctx, tabID)
	if err != nil {
		return nil, err
	}

	// Post-flight: scan text for injection patterns.
	scanResult := s.guard.ScanContent(result.Text)
	if scanResult.Blocked {
		return nil, fmt.Errorf("content blocked by IDPI scanner: %s", scanResult.Reason)
	}
	if scanResult.Threat {
		slog.Warn("IDPI content warning on text", "engine", result.Engine, "reason", scanResult.Reason)
	}

	// Wrap content with trust-boundary markers.
	if s.wrap {
		result.Text = s.guard.WrapContent(result.Text, result.URL)
	}

	return result, nil
}

func (s *SafeEngine) Click(ctx context.Context, tabID, ref string) error {
	return s.inner.Click(ctx, tabID, ref)
}

func (s *SafeEngine) Type(ctx context.Context, tabID, ref, text string) error {
	return s.inner.Type(ctx, tabID, ref, text)
}

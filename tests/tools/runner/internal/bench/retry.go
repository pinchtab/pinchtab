package bench

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const (
	MaxRetryAttempts  = 3
	MaxRetryBackoff   = 15 * time.Second
	InitialRetryDelay = 2 * time.Second
)

type RetryConfig struct {
	MaxAttempts  int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Clock        func() time.Time
	Sleep        func(time.Duration)
}

func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  MaxRetryAttempts,
		InitialDelay: InitialRetryDelay,
		MaxDelay:     MaxRetryBackoff,
		Clock:        time.Now,
		Sleep:        time.Sleep,
	}
}

type DoFunc func() (*http.Response, error)

func DoWithRetry(ctx context.Context, cfg RetryConfig, do DoFunc) (*http.Response, error) {
	var lastErr error
	delay := cfg.InitialDelay

	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		resp, err := do()
		if err != nil {
			lastErr = err
			if attempt < cfg.MaxAttempts-1 {
				cfg.Sleep(delay)
				delay = min(delay*2, cfg.MaxDelay)
			}
			continue
		}

		if !isRetryable(resp.StatusCode) || attempt == cfg.MaxAttempts-1 {
			return resp, nil
		}

		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()

		if retryDelay := parseRetryAfter(resp.Header.Get("Retry-After"), cfg.Clock()); retryDelay > 0 {
			delay = min(retryDelay, cfg.MaxDelay)
		}
		cfg.Sleep(delay)
		delay = min(delay*2, cfg.MaxDelay)
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

func isRetryable(statusCode int) bool {
	switch statusCode {
	case 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	if value == "" {
		return 0
	}

	if seconds, err := strconv.Atoi(value); err == nil {
		return time.Duration(seconds) * time.Second
	}

	if t, err := http.ParseTime(value); err == nil {
		return t.Sub(now)
	}

	return 0
}

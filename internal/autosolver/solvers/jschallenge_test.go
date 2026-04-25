package solvers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/autosolver"
)

type jsMockPage struct {
	title   string
	url     string
	htmlSeq []string
	idx     int
}

func (m *jsMockPage) URL() string   { return m.url }
func (m *jsMockPage) Title() string { return m.title }
func (m *jsMockPage) Screenshot() ([]byte, error) {
	return nil, nil
}
func (m *jsMockPage) HTML() (string, error) {
	if len(m.htmlSeq) == 0 {
		return "", nil
	}
	if m.idx >= len(m.htmlSeq) {
		return m.htmlSeq[len(m.htmlSeq)-1], nil
	}
	h := m.htmlSeq[m.idx]
	m.idx++
	return h, nil
}

type jsMockExecutor struct {
	clicks      int
	evals       int
	waits       int
	clickErr    error
	evaluateErr error
	waitErr     error
}

func (m *jsMockExecutor) Click(_ context.Context, _, _ float64) error {
	m.clicks++
	return m.clickErr
}
func (m *jsMockExecutor) Type(_ context.Context, _ string) error { return nil }
func (m *jsMockExecutor) WaitFor(_ context.Context, _ string, _ time.Duration) error {
	m.waits++
	return m.waitErr
}
func (m *jsMockExecutor) Evaluate(_ context.Context, _ string, _ interface{}) error {
	m.evals++
	return m.evaluateErr
}
func (m *jsMockExecutor) Navigate(_ context.Context, _ string) error { return nil }

func TestJSChallenge_CanHandle(t *testing.T) {
	s := &JSChallenge{}
	page := &jsMockPage{
		title: "Browser Integrity Check",
		url:   "https://example.com/challenge",
		htmlSeq: []string{
			`<script>window._cf_chl_opt = {}</script>`,
		},
	}
	ok, err := s.CanHandle(context.Background(), page)
	if err != nil {
		t.Fatalf("CanHandle error: %v", err)
	}
	if !ok {
		t.Fatal("expected CanHandle=true")
	}
}

func TestJSChallenge_SolveResolves(t *testing.T) {
	s := &JSChallenge{}
	page := &jsMockPage{
		title: "Browser Integrity Check",
		url:   "https://example.com/challenge",
		htmlSeq: []string{
			`<script>window._cf_chl_opt = {}</script>`,
			`<html><body>still loading</body></html>`,
			`<html><body>ok</body></html>`,
		},
	}
	executor := &jsMockExecutor{}

	result, err := s.Solve(context.Background(), page, executor)
	if err != nil {
		t.Fatalf("Solve error: %v", err)
	}
	if !result.Solved {
		t.Fatalf("expected solved result, got error=%q", result.Error)
	}
	if executor.waits == 0 {
		t.Fatal("expected wait to be called")
	}
}

func TestJSChallenge_SolveWaitError(t *testing.T) {
	s := &JSChallenge{}
	page := &jsMockPage{
		title: "Browser Integrity Check",
		url:   "https://example.com/challenge",
		htmlSeq: []string{
			`<script>window._cf_chl_opt = {}</script>`,
		},
	}
	executor := &jsMockExecutor{waitErr: errors.New("wait fail")}

	result, err := s.Solve(context.Background(), page, executor)
	if err == nil {
		t.Fatal("expected wait error")
	}
	if result == nil || result.Error == "" {
		t.Fatal("expected result error message")
	}
}

func TestJSChallenge_SolveContextCancel(t *testing.T) {
	s := &JSChallenge{}
	page := &jsMockPage{
		title: "Browser Integrity Check",
		url:   "https://example.com/challenge",
		htmlSeq: []string{
			`<script>window._cf_chl_opt = {}</script>`,
			`<script>window._cf_chl_opt = {}</script>`,
			`<script>window._cf_chl_opt = {}</script>`,
		},
	}
	executor := &jsMockExecutor{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := s.Solve(ctx, page, executor)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
	if result == nil || result.Error == "" {
		t.Fatal("expected result with context error")
	}
}

func TestJSChallenge_UsesSharedDetection(t *testing.T) {
	intent := autosolver.DetectChallengeIntent(
		"Browser Integrity Check",
		"https://example.com/challenge",
		`<script>window._cf_chl_opt = {}</script>`,
	)
	if intent == nil {
		t.Fatal("expected shared detection intent")
	}
	if intent.ChallengeType != "custom-js" {
		t.Fatalf("expected custom-js challenge type, got %q", intent.ChallengeType)
	}
}

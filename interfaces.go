package main

import (
	"context"
	"net/http"

	"github.com/chromedp/cdproto/target"
)

// Compile-time interface satisfaction checks.
var (
	_ ProfileService      = (*ProfileManager)(nil)
	_ OrchestratorService = (*Orchestrator)(nil)
)

// BridgeAPI abstracts browser tab operations for handler testing.
type BridgeAPI interface {
	TabContext(tabID string) (ctx context.Context, resolvedID string, err error)
	ListTargets() ([]*target.Info, error)
	CreateTab(url string) (tabID string, ctx context.Context, cancel context.CancelFunc, err error)
	CloseTab(tabID string) error

	GetRefCache(tabID string) *refCache
	SetRefCache(tabID string, cache *refCache)
	DeleteRefCache(tabID string)
}

// TabInfo describes a browser tab.
type TabInfo struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

// ProfileService abstracts profile management operations.
type ProfileService interface {
	RegisterHandlers(mux *http.ServeMux)
	List() ([]ProfileInfo, error)
	Create(name string) error
	Import(name, sourcePath string) error
	Reset(name string) error
	Delete(name string) error
	Logs(name string, limit int) []ActionRecord
	Analytics(name string) AnalyticsReport
	RecordAction(profile string, record ActionRecord)
}

// OrchestratorService abstracts instance orchestration operations.
type OrchestratorService interface {
	RegisterHandlers(mux *http.ServeMux)
	Launch(name, port string, headless bool) (*Instance, error)
	Stop(id string) error
	StopProfile(name string) error
	List() []Instance
	Logs(id string) (string, error)
	FirstRunningURL() string
	AllTabs() []instanceTab
	ScreencastURL(instanceID, tabID string) string
	Shutdown()
	ForceShutdown()
}

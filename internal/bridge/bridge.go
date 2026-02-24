package bridge

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/uameta"
)

type TabEntry struct {
	Ctx      context.Context
	Cancel   context.CancelFunc
	Accessed bool
}

type RefCache struct {
	Refs  map[string]int64
	Nodes []A11yNode
}

type Bridge struct {
	AllocCtx   context.Context
	BrowserCtx context.Context
	Config     *config.RuntimeConfig
	*TabManager
	StealthScript string
	Actions       map[string]ActionFunc
	Locks         *LockManager
}

func New(allocCtx, browserCtx context.Context, cfg *config.RuntimeConfig) *Bridge {
	b := &Bridge{
		AllocCtx:   allocCtx,
		BrowserCtx: browserCtx,
		Config:     cfg,
	}
	if cfg != nil {
		b.TabManager = NewTabManager(browserCtx, cfg, b.tabSetup)
	}
	b.Locks = NewLockManager()
	return b
}

func (b *Bridge) injectStealth(ctx context.Context) {
	if b.StealthScript == "" {
		return
	}
	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(b.StealthScript).Do(ctx)
			return err
		}),
	); err != nil {
		slog.Warn("stealth injection failed", "err", err)
	}
}

func (b *Bridge) tabSetup(ctx context.Context) {
	if override := uameta.Build(b.Config.UserAgent, b.Config.ChromeVersion); override != nil {
		if err := chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
			return override.Do(c)
		})); err != nil {
			slog.Warn("ua override failed on tab setup", "err", err)
		}
	}
	b.injectStealth(ctx)
	if b.Config.NoAnimations {
		b.InjectNoAnimations(ctx)
	}
}

func (b *Bridge) Lock(tabID, owner string, ttl time.Duration) error {
	return b.Locks.TryLock(tabID, owner, ttl)
}

func (b *Bridge) Unlock(tabID, owner string) error {
	return b.Locks.Unlock(tabID, owner)
}

func (b *Bridge) TabLockInfo(tabID string) *LockInfo {
	return b.Locks.Get(tabID)
}

func (b *Bridge) BrowserContext() context.Context {
	return b.BrowserCtx
}

func (b *Bridge) ExecuteAction(ctx context.Context, kind string, req ActionRequest) (map[string]any, error) {
	fn, ok := b.Actions[kind]
	if !ok {
		return nil, fmt.Errorf("unknown action: %s", kind)
	}
	return fn(ctx, req)
}

func (b *Bridge) AvailableActions() []string {
	keys := make([]string, 0, len(b.Actions))
	for k := range b.Actions {
		keys = append(keys, k)
	}
	return keys
}

// ActionFunc is the type for action handlers.
type ActionFunc func(ctx context.Context, req ActionRequest) (map[string]any, error)

// ActionRequest defines the parameters for a browser action.
type ActionRequest struct {
	TabID    string `json:"tabId"`
	Kind     string `json:"kind"`
	Ref      string `json:"ref"`
	Selector string `json:"selector"`
	Text     string `json:"text"`
	Key      string `json:"key"`
	Value    string `json:"value"`
	NodeID   int64  `json:"nodeId"`
	ScrollX  int    `json:"scrollX"`
	ScrollY  int    `json:"scrollY"`
	WaitNav  bool   `json:"waitNav"`
	Fast     bool   `json:"fast"`
}

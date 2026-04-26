package bridge

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	cdp "github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	internalurls "github.com/pinchtab/pinchtab/internal/urls"
)

func shouldBlockPopupTarget(info *target.Info) bool {
	return info != nil && info.Type == TargetTypePage && info.OpenerID != ""
}

func (tm *TabManager) StartBrowserGuards() {
	if tm == nil || tm.browserCtx == nil {
		return
	}

	tm.guardOnce.Do(func() {
		if err := chromedp.Run(tm.browserCtx, chromedp.ActionFunc(func(ctx context.Context) error {
			c := chromedp.FromContext(ctx)
			if c == nil || c.Browser == nil {
				return fmt.Errorf("no browser executor")
			}
			return target.SetDiscoverTargets(true).Do(cdp.WithExecutor(ctx, c.Browser))
		})); err != nil {
			slog.Warn("browser popup guard unavailable", "err", err)
			return
		}

		chromedp.ListenBrowser(tm.browserCtx, func(ev any) {
			created, ok := ev.(*target.EventTargetCreated)
			if !ok || !shouldBlockPopupTarget(created.TargetInfo) {
				return
			}

			info := created.TargetInfo
			go tm.closePopupTarget(info.TargetID, info.OpenerID, info.URL)
		})
	})
}

func (tm *TabManager) closePopupTarget(targetID, openerID target.ID, url string) {
	closeCtx, cancel := context.WithTimeout(tm.browserCtx, 5*time.Second)
	defer cancel()
	logURL := internalurls.RedactForLog(url)

	execCtx, err := browserExecutorContext(closeCtx)
	if err != nil {
		slog.Debug("popup close skipped", "targetId", targetID, "openerId", openerID, "url", logURL, "err", err)
		return
	}

	if err := target.CloseTarget(targetID).Do(execCtx); err != nil {
		slog.Debug("popup close failed", "targetId", targetID, "openerId", openerID, "url", logURL, "err", err)
		return
	}

	slog.Info("blocked popup target", "targetId", targetID, "openerId", openerID, "url", logURL)
}

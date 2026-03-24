package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/stealth"
)

func (b *Bridge) installWorkerStealthParity(ctx context.Context) {
	if b == nil || b.Config == nil {
		return
	}

	chromedp.ListenTarget(ctx, func(ev any) {
		attached, ok := ev.(*target.EventAttachedToTarget)
		if !ok || attached.TargetInfo == nil || !strings.Contains(attached.TargetInfo.Type, "worker") {
			return
		}

		targetID := string(attached.TargetInfo.TargetID)
		if _, loaded := b.workerStealthTargets.LoadOrStore(targetID, struct{}{}); loaded {
			return
		}

		go b.applyWorkerStealth(ctx, attached.TargetInfo.TargetID, attached.TargetInfo.Type)
	})
}

func (b *Bridge) applyWorkerStealth(parent context.Context, targetID target.ID, targetType string) {
	workerCtx, cancel := chromedp.NewContext(parent, chromedp.WithTargetID(targetID))
	defer cancel()

	runCtx, runCancel := context.WithTimeout(workerCtx, 5*time.Second)
	defer runCancel()

	ua := ""
	if b.StealthBundle != nil {
		ua = b.StealthBundle.LaunchUserAgent()
	}

	if err := chromedp.Run(runCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return stealth.ApplyTargetEmulation(ctx, b.Config, ua)
		}),
		chromedp.Evaluate(workerStealthParityScript(stealth.BuildPersona(ua, b.Config.ChromeVersion)), nil),
	); err != nil {
		slog.Debug("worker stealth parity failed", "targetId", targetID, "targetType", targetType, "err", err)
	}
}

func workerStealthParityScript(persona stealth.BrowserPersona) string {
	languagesJSON := safeJSONStringArray(persona.Languages, `["en-US","en"]`)

	return fmt.Sprintf(`(() => {
  try {
    const nav = self.navigator;
    if (!nav) return;
    const target = Object.getPrototypeOf(nav) || nav;
    const define = (name, getter) => {
      try { Object.defineProperty(target, name, { get: getter, configurable: true }); } catch (e) {}
      try { Object.defineProperty(nav, name, { get: getter, configurable: true }); } catch (e) {}
    };
    const ua = %q || nav.userAgent || '';
    const platform = %q || nav.platform || '';
    const language = %q || nav.language || 'en-US';
    const languages = JSON.parse(%q);
    if (ua) define('userAgent', () => ua);
    if (platform) define('platform', () => platform);
    define('language', () => language);
    define('languages', () => languages.slice());
    define('webdriver', () => false);
  } catch (e) {}
})()`, persona.UserAgent, persona.NavigatorPlatform, persona.Language, languagesJSON)
}

// safeJSONStringArray marshals a string slice to JSON and validates the output
// is a JSON array of strings. Returns fallback on any error.
func safeJSONStringArray(values []string, fallback string) string {
	data, err := json.Marshal(values)
	if err != nil {
		return fallback
	}
	// Round-trip to verify output is a valid string array
	var check []string
	if json.Unmarshal(data, &check) != nil {
		return fallback
	}
	return string(data)
}

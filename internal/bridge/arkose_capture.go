package bridge

import (
	"context"
	"log/slog"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// arkoseCaptureScript runs at document-start and transparently captures the
// per-session Arkose FunCaptcha data the host page hands to ArkoseEnforcement —
// the `data.blob`, the public key, and the service URL — onto window.__ptArkose,
// and preserves the host page's onCompleted callback on window.__ptArkoseOnCompleted.
//
// The capsolver autosolver reads window.__ptArkose via Evaluate and forwards the
// blob to CapSolver's FunCaptchaTaskProxyLess task; modern Arkose deployments
// (e.g. LinkedIn) reject solves that lack the per-session blob, which is generated
// by the site's JS at runtime and never present in static HTML. After the token is
// solved, delivering it through window.__ptArkoseOnCompleted advances the site's
// own completion flow. The hook is inert on non-Arkose pages — nothing assigns
// window.ArkoseEnforcement there, so the accessor never fires.
//
// Limitation: the accessor is installed before the site's api.js runs, so the
// property is observably "present" (a getter returning undefined). A site that
// gates its init on `'ArkoseEnforcement' in window` could skip assigning — in
// which case neither the capture nor the page's own widget initializes. A fully
// transparent capture would intercept at the CDP network layer instead; this
// in-page hook is the pragmatic tradeoff.
const arkoseCaptureScript = `(function(){
  if (window.__ptArkoseHook) return;
  try { Object.defineProperty(window, '__ptArkoseHook', {value:true, configurable:true}); } catch(e){ return; }
  function grab(cfg){
    try{
      if (cfg && cfg.data && typeof cfg.data.blob === 'string'){
        window.__ptArkose = { blob: cfg.data.blob, pk: cfg.public_key || cfg.publicKey || '', surl: cfg.surl || '' };
      }
      if (cfg && typeof cfg.onCompleted === 'function'){ window.__ptArkoseOnCompleted = cfg.onCompleted; }
    }catch(e){}
  }
  var real;
  try{
    Object.defineProperty(window, 'ArkoseEnforcement', {
      configurable: true, enumerable: true,
      get: function(){ return real; },
      set: function(v){
        try{
          if (typeof v === 'function'){
            // Idempotent: a re-assignment of ArkoseEnforcement must not wrap an
            // already-wrapped setConfig (that makes origSet the wrapper itself →
            // infinite recursion on the next setConfig call).
            if (v.prototype && typeof v.prototype.setConfig === 'function' && !v.prototype.setConfig.__ptWrapped){
              var origSet = v.prototype.setConfig;
              var wrappedSet = function(cfg){ grab(cfg); return origSet.apply(this, arguments); };
              wrappedSet.__ptWrapped = true;
              v.prototype.setConfig = wrappedSet;
            }
            // newTarget defaults to v so new.target === v inside the real
            // constructor (some Arkose builds tamper-check it); instanceof still
            // holds because Wrapped.prototype === v.prototype.
            var Wrapped = function(cfg){ grab(cfg); return Reflect.construct(v, arguments); };
            Wrapped.prototype = v.prototype;
            Object.setPrototypeOf(Wrapped, v);
            real = Wrapped;
            return;
          }
        }catch(e){}
        real = v;
      }
    });
  }catch(e){}
})();`

// injectArkoseCapture installs the document-start Arkose blob-capture hook on the
// current target. Mirrors injectStealth; failures are non-fatal.
func (b *Bridge) injectArkoseCapture(ctx context.Context) {
	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(arkoseCaptureScript).Do(ctx)
			return err
		}),
	); err != nil {
		slog.Warn("arkose capture injection failed", "err", err)
	}
}

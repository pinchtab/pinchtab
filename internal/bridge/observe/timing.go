package observe

// TimingMetrics holds navigation timing, Core Web Vitals, and the resource
// loading breakdown for a page. All durations are milliseconds relative to
// navigation start.
type TimingMetrics struct {
	TTFBMs             float64 `json:"ttfbMs"`
	FCPMs              float64 `json:"fcpMs"`
	LCPMs              float64 `json:"lcpMs"`
	CLS                float64 `json:"cls"`
	DOMContentLoadedMs float64 `json:"domContentLoadedMs"`
	LoadMs             float64 `json:"loadMs"`
	ResourceCount      int     `json:"resourceCount"`
	TransferSizeBytes  int64   `json:"transferSizeBytes"`
}

// rawTiming is the JSON shape TimingScript resolves to.
type rawTiming struct {
	TTFB             float64 `json:"ttfb"`
	FCP              float64 `json:"fcp"`
	LCP              float64 `json:"lcp"`
	CLS              float64 `json:"cls"`
	DOMContentLoaded float64 `json:"domContentLoaded"`
	Load             float64 `json:"load"`
	ResourceCount    float64 `json:"resourceCount"`
	TransferSize     float64 `json:"transferSize"`
}

// TimingScript gathers performance data in the page and resolves to the
// rawTiming shape. It waits for the load event when the page is still
// loading and for the first paint entry (paint metrics only fire on visible
// pages — the caller must bring the tab to front first), then reads
// navigation/paint/resource entries plus buffered largest-contentful-paint
// and layout-shift observer entries. Evaluate it with AwaitPromise.
const TimingScript = `(() => {
  const whenLoaded = document.readyState === 'complete'
    ? Promise.resolve()
    : new Promise((resolve) => window.addEventListener('load', resolve, { once: true }));
  const whenLoadTimed = () => new Promise((resolve) => {
    const started = performance.now();
    const check = () => {
      const nav = performance.getEntriesByType('navigation')[0];
      if ((nav && nav.loadEventEnd > 0) || performance.now() - started > 2000) resolve();
      else setTimeout(check, 50);
    };
    check();
  });
  const whenPainted = () => new Promise((resolve) => {
    try {
      const po = new PerformanceObserver(() => { po.disconnect(); resolve(); });
      po.observe({ type: 'paint', buffered: true });
      setTimeout(() => { po.disconnect(); resolve(); }, 1500);
    } catch (err) { resolve(); }
  });
  const collect = (type) => new Promise((resolve) => {
    const entries = [];
    try {
      const po = new PerformanceObserver((list) => entries.push(...list.getEntries()));
      po.observe({ type, buffered: true });
      setTimeout(() => { entries.push(...po.takeRecords()); po.disconnect(); resolve(entries); }, 60);
    } catch (err) { resolve(entries); }
  });
  return whenLoaded
    .then(whenLoadTimed)
    .then(whenPainted)
    .then(() => new Promise((resolve) => setTimeout(resolve, 60)))
    .then(() => Promise.all([collect('largest-contentful-paint'), collect('layout-shift')]))
    .then(([lcps, shifts]) => {
      const nav = performance.getEntriesByType('navigation')[0];
      const fcp = performance.getEntriesByType('paint').find((e) => e.name === 'first-contentful-paint');
      const resources = performance.getEntriesByType('resource');
      return {
        ttfb: nav ? nav.responseStart : 0,
        domContentLoaded: nav ? nav.domContentLoadedEventEnd : 0,
        load: nav ? nav.loadEventEnd : 0,
        fcp: fcp ? fcp.startTime : 0,
        lcp: lcps.length ? lcps[lcps.length - 1].startTime : 0,
        cls: shifts.reduce((sum, e) => (e.hadRecentInput ? sum : sum + e.value), 0),
        resourceCount: resources.length,
        transferSize: (nav ? nav.transferSize || 0 : 0) + resources.reduce((s, e) => s + (e.transferSize || 0), 0),
      };
    });
})()`

// toTimingMetrics maps the raw in-page payload to the public metric shape,
// clamping nonsensical negative values to zero.
func toTimingMetrics(raw rawTiming) *TimingMetrics {
	clamp := func(v float64) float64 {
		if v < 0 {
			return 0
		}
		return v
	}
	return &TimingMetrics{
		TTFBMs:             clamp(raw.TTFB),
		FCPMs:              clamp(raw.FCP),
		LCPMs:              clamp(raw.LCP),
		CLS:                clamp(raw.CLS),
		DOMContentLoadedMs: clamp(raw.DOMContentLoaded),
		LoadMs:             clamp(raw.Load),
		ResourceCount:      int(clamp(raw.ResourceCount)),
		TransferSizeBytes:  int64(clamp(raw.TransferSize)),
	}
}

// CollectTiming evaluates TimingScript through eval (a bridge Evaluate bound
// to a tab context with AwaitPromise) and maps the result.
func CollectTiming(eval func(expression string, result any) error) (*TimingMetrics, error) {
	var raw rawTiming
	if err := eval(TimingScript, &raw); err != nil {
		return nil, err
	}
	return toTimingMetrics(raw), nil
}

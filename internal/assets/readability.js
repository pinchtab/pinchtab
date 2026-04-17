(() => {
  const strip = ['nav', 'footer', 'aside', 'header', '[role="navigation"]',
    '[role="banner"]', '[role="contentinfo"]', '[aria-hidden="true"]',
    '.ad', '.ads', '.advertisement', '.sidebar', '.cookie-banner',
    '#cookie-consent', '.popup', '.modal',
    '#SIvCob', '[data-locale-picker]', '[role="listbox"]',
    '#Lb4nn', '.language-selector', '.locale-selector',
    '[data-language-picker]', '#langsec-button'];

  // Pick the first VISIBLE candidate root. Pages may contain multiple
  // <main> or [role="main"] elements and toggle between them with
  // display:none (common SPA pattern). querySelector() returns the first
  // in document order regardless of visibility, which produces stale
  // content after in-place DOM updates.
  const isVisible = (el) => {
    if (!el || !el.isConnected) return false;
    const style = window.getComputedStyle(el);
    if (style.display === 'none' || style.visibility === 'hidden') return false;
    const rect = el.getBoundingClientRect();
    return rect.width > 0 || rect.height > 0;
  };

  const firstVisible = (selector) => {
    const nodes = document.querySelectorAll(selector);
    for (const n of nodes) {
      if (isVisible(n)) return n;
    }
    return null;
  };

  let root = firstVisible('article') ||
             firstVisible('[role="main"]') ||
             firstVisible('main');

  if (!root) {
    root = document.body.cloneNode(true);
    for (const sel of strip) {
      root.querySelectorAll(sel).forEach(el => el.remove());
    }
  } else {
    root = root.cloneNode(true);
  }

  root.querySelectorAll('script, style, noscript, svg, [hidden]').forEach(el => el.remove());

  return root.innerText.replace(/\n{3,}/g, '\n\n').trim();
})()

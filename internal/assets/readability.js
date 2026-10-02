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

  const source = firstVisible('article') ||
                 firstVisible('[role="main"]') ||
                 firstVisible('main');
  const live = source || document.body;
  const root = live.cloneNode(true);

  // cloneNode does not carry a select's current selection, so read it from the live
  // select at the same document-order position, before anything is stripped from the clone.
  const liveSelects = live.querySelectorAll('select');
  const chosenLabels = new Map();
  root.querySelectorAll('select').forEach((select, i) => {
    chosenLabels.set(select, Array.from(liveSelects[i].selectedOptions, o => o.label).join(', '));
  });

  const unrendered = new Set();
  const invisible = new Set();
  const markHidden = (liveEl, cloneEl) => {
    const style = window.getComputedStyle(liveEl);
    if (style.display === 'none') {
      unrendered.add(cloneEl);
      return;
    }
    if (style.visibility !== 'visible') invisible.add(cloneEl);
    for (let i = 0; i < liveEl.children.length; i++) {
      markHidden(liveEl.children[i], cloneEl.children[i]);
    }
  };
  markHidden(live, root);
  unrendered.forEach(el => el.remove());

  if (!source) {
    for (const sel of strip) {
      root.querySelectorAll(sel).forEach(el => el.remove());
    }
  }

  root.querySelectorAll('script, style, noscript, svg, [hidden]').forEach(el => el.remove());

  // root is always a CLONE, so it is detached, and innerText on an unrendered node
  // has no layout to consult — it degrades to textContent, which concatenates
  // descendant text with no separator at all. That is how a status cell and a
  // timestamp cell came back as one unparseable number, and how two adjacent
  // paragraphs came back as one word.
  //
  // serialize walks the clone instead and supplies the boundaries layout would
  // have: a newline between blocks, a tab between table cells. It inserts a
  // separator ONLY where the text either side would otherwise fuse, so a document
  // whose source already carries whitespace between its tags serialises exactly as
  // it did before and nothing reflows.
  //
  // Every mutation above and every read below happen on the clone, which is never
  // inserted into the document — that is what guarantees a read cannot alter the
  // page an agent is driving. Do not "fix" this by attaching the clone off-screen
  // to give it layout: that fires the page's mutation observers and re-requests
  // every image and iframe in the copy.
  const BLOCKS = new Set(['ADDRESS', 'ARTICLE', 'ASIDE', 'BLOCKQUOTE', 'DD', 'DETAILS',
    'DIALOG', 'DIV', 'DL', 'DT', 'FIELDSET', 'FIGCAPTION', 'FIGURE', 'FOOTER', 'FORM',
    'H1', 'H2', 'H3', 'H4', 'H5', 'H6', 'HEADER', 'HGROUP', 'HR', 'LI', 'MAIN', 'NAV',
    'OL', 'P', 'PRE', 'SECTION', 'TABLE', 'TBODY', 'TFOOT', 'THEAD', 'TR', 'UL']);

  let out = '';
  let pending = '';
  // A boundary QUEUES a separator rather than writing one. It is written only when the
  // next text arrives with nothing separating it from what came before — which is what
  // keeps a document that already spaces its tags byte-identical to the old output. A
  // newline outranks a tab, so the first cell of a row starts a line rather than
  // continuing one.
  const queue = (sep) => {
    if (sep === '\n' || pending === '') pending = sep;
  };
  const write = (data) => {
    if (data === '') return;
    if (pending && out !== '' && !/\s$/.test(out) && !/^\s/.test(data)) out += pending;
    pending = '';
    out += data;
  };
  const serialize = (node) => {
    if (node.nodeType === Node.TEXT_NODE) {
      if (!invisible.has(node.parentNode)) write(node.data);
      return;
    }
    if (node.nodeType !== Node.ELEMENT_NODE) return;
    if (node.tagName === 'BR') {
      queue('\n');
      return;
    }
    if (node.tagName === 'SELECT') {
      queue(' ');
      if (!invisible.has(node)) write(chosenLabels.get(node));
      queue(' ');
      return;
    }
    const block = BLOCKS.has(node.tagName);
    const button = node.tagName === 'BUTTON';
    if (button) queue(' ');
    if (block) queue('\n');
    if (node.tagName === 'TD' || node.tagName === 'TH') queue('\t');
    for (const child of node.childNodes) serialize(child);
    if (block) queue('\n');
    if (button) queue(' ');
  };
  if (!unrendered.has(root)) serialize(root);

  return out.replace(/\n{3,}/g, '\n\n').trim();
})()

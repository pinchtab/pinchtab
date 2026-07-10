package cdptk

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/chromedp/chromedp"
)

// InteractiveOverlayRootID is the stable DOM id for the persistent, clickable
// annotation layer. It is intentionally distinct from OverlayRootID so the
// transient screenshot overlay and the persistent interactive overlay never
// clobber each other: taking an annotated screenshot must not wipe a
// human-facing overlay a user injected with `annotate`.
const InteractiveOverlayRootID = "__pinchtab_annotations_interactive__"

// interactiveOverlayScript builds the persistent overlay. Unlike
// InjectAnnotationOverlay (which is torn down right after a screenshot), this
// overlay stays on the page and each label is clickable: clicking copies a
// reference block for that element to the clipboard so a human can paste it
// into an LLM ("fix this: <ref>, <css>, ...").
//
// {{DATA}} is the JSON item array; {{ROOT}} is the overlay root id. The body is
// kept free of Go fmt verbs so we can substitute via a plain replacer.
const interactiveOverlayScript = `(function(items, rootId) {
	var prev = document.getElementById(rootId);
	if (prev) prev.remove();
	var sx = window.scrollX || window.pageXOffset || 0;
	var sy = window.scrollY || window.pageYOffset || 0;
	window.__ptLastCopied = null;

	function cssPath(el) {
		if (!el || el.nodeType !== 1) return '';
		var parts = [];
		while (el && el.nodeType === 1 && el !== document.documentElement) {
			var sel = el.nodeName.toLowerCase();
			if (el.id) { parts.unshift('#' + CSS.escape(el.id)); break; }
			var p = el.parentElement;
			if (p) {
				var same = Array.prototype.filter.call(p.children, function(c) { return c.nodeName === el.nodeName; });
				if (same.length > 1) sel += ':nth-of-type(' + (same.indexOf(el) + 1) + ')';
			}
			parts.unshift(sel);
			el = el.parentElement;
		}
		return parts.join(' > ');
	}
	function xPath(el) {
		var parts = [];
		while (el && el.nodeType === 1) {
			var i = 1, s = el.previousElementSibling;
			while (s) { if (s.nodeName === el.nodeName) i++; s = s.previousElementSibling; }
			parts.unshift(el.nodeName.toLowerCase() + '[' + i + ']');
			el = el.parentElement;
		}
		return '/' + parts.join('/');
	}
	// resolveEl maps an annotation box back to the live DOM element. Boxes carry
	// pointer-events:none so elementFromPoint at the box centre passes through to
	// the underlying element; we then walk up to the ancestor whose rect best
	// matches the annotation box, since the point may land on a child node.
	function resolveEl(it) {
		var cx = it.x + sx - (window.scrollX || window.pageXOffset || 0) + it.w / 2;
		var cy = it.y + sy - (window.scrollY || window.pageYOffset || 0) + it.h / 2;
		var el = document.elementFromPoint(cx, cy);
		if (!el) return null;
		var best = el, bestDiff = Infinity, node = el;
		for (var k = 0; k < 6 && node; k++) {
			var r = node.getBoundingClientRect();
			var diff = Math.abs(r.width - it.w) + Math.abs(r.height - it.h);
			if (diff < bestDiff) { bestDiff = diff; best = node; }
			node = node.parentElement;
		}
		return best;
	}
	function fallbackCopy(t) {
		var ta = document.createElement('textarea');
		ta.value = t; ta.style.position = 'fixed'; ta.style.opacity = '0';
		document.body.appendChild(ta); ta.focus(); ta.select();
		try { document.execCommand('copy'); } catch (e) {}
		ta.remove();
		return Promise.resolve();
	}
	function copyText(t) {
		window.__ptLastCopied = t;
		if (navigator.clipboard && navigator.clipboard.writeText) {
			return navigator.clipboard.writeText(t).catch(function() { return fallbackCopy(t); });
		}
		return fallbackCopy(t);
	}
	function reference(it) {
		var el = resolveEl(it);
		var css = el ? cssPath(el) : '(unresolved)';
		var xp = el ? xPath(el) : '';
		var head = it.ref;
		if (it.role) head += ' — ' + it.role;
		if (it.name) head += ' "' + it.name + '"';
		return 'Page: ' + document.title + ' (' + location.href + ')\n' +
			'Element: ' + head + '\n' +
			'CSS: ' + css + '\n' +
			'XPath: ' + xp;
	}

	var root = document.createElement('div');
	root.id = rootId;
	root.style.position = 'absolute';
	root.style.top = '0';
	root.style.left = '0';
	root.style.width = '0';
	root.style.height = '0';
	root.style.pointerEvents = 'none';
	root.style.zIndex = '2147483646';

	items.forEach(function(it) {
		var box = document.createElement('div');
		box.style.position = 'absolute';
		box.style.left = (it.x + sx) + 'px';
		box.style.top = (it.y + sy) + 'px';
		box.style.width = it.w + 'px';
		box.style.height = it.h + 'px';
		box.style.boxSizing = 'border-box';
		box.style.border = '2px solid rgba(255, 51, 102, 0.95)';
		box.style.borderRadius = '2px';
		box.style.pointerEvents = 'none';

		var label = document.createElement('div');
		label.textContent = it.ref;
		label.title = 'Click to copy a reference for ' + it.ref;
		label.style.position = 'absolute';
		label.style.left = (it.x + sx) + 'px';
		label.style.top = Math.max(0, it.y + sy - 16) + 'px';
		label.style.padding = '0 5px';
		label.style.background = 'rgba(255, 51, 102, 0.95)';
		label.style.color = '#fff';
		label.style.font = '600 12px/16px ui-monospace, SFMono-Regular, Menlo, Consolas, monospace';
		label.style.borderRadius = '2px 2px 0 0';
		label.style.whiteSpace = 'nowrap';
		label.style.userSelect = 'none';
		label.style.cursor = 'pointer';
		label.style.pointerEvents = 'auto';
		label.addEventListener('click', function(ev) {
			ev.preventDefault();
			ev.stopPropagation();
			copyText(reference(it)).then(function() {
				var orig = label.textContent;
				label.textContent = '✓ ' + it.ref;
				label.style.background = 'rgba(40, 200, 90, 0.95)';
				setTimeout(function() {
					label.textContent = orig;
					label.style.background = 'rgba(255, 51, 102, 0.95)';
				}, 1300);
			});
		});

		root.appendChild(box);
		root.appendChild(label);
	});
	document.documentElement.appendChild(root);
	return items.length;
})({{DATA}}, "{{ROOT}}")`

// InjectInteractiveOverlay paints a persistent overlay whose labels are
// clickable. Coordinates are viewport-relative (as produced by
// AnnotationRectForNode); the page adds scrollX/scrollY so boxes track the
// document while scrolling. The overlay is NOT auto-removed — callers remove it
// via RemoveInteractiveOverlay.
func InjectInteractiveOverlay(ctx context.Context, items []AnnotationItem) error {
	if len(items) == 0 {
		return nil
	}
	type overlayItem struct {
		Ref  string  `json:"ref"`
		Role string  `json:"role,omitempty"`
		Name string  `json:"name,omitempty"`
		X    float64 `json:"x"`
		Y    float64 `json:"y"`
		W    float64 `json:"w"`
		H    float64 `json:"h"`
	}
	payload := make([]overlayItem, len(items))
	for i, it := range items {
		payload[i] = overlayItem{
			Ref:  it.Ref,
			Role: it.Role,
			Name: it.Name,
			X:    it.Box.X,
			Y:    it.Box.Y,
			W:    it.Box.W,
			H:    it.Box.H,
		}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	script := strings.NewReplacer(
		"{{DATA}}", string(encoded),
		"{{ROOT}}", InteractiveOverlayRootID,
	).Replace(interactiveOverlayScript)
	return chromedp.Run(ctx, chromedp.Evaluate(script, nil))
}

// RemoveInteractiveOverlay removes the persistent overlay if present. Safe to
// call when no overlay exists.
func RemoveInteractiveOverlay(ctx context.Context) error {
	script := `(function(rootId) {
		var el = document.getElementById(rootId);
		if (el) el.remove();
		return true;
	})("` + InteractiveOverlayRootID + `")`
	return chromedp.Run(ctx, chromedp.Evaluate(script, nil))
}

package cdpops

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/cdptk"
)

var (
	ErrElementOccluded  = errors.New("element is occluded")
	ErrElementHidden    = errors.New("element is hidden")
	ErrElementBlocked   = errors.New("element is blocked from pointer interaction")
	ErrElementOffscreen = errors.New("element center is outside viewport")
)

type pointerProbe struct {
	X            float64  `json:"x"`
	Y            float64  `json:"y"`
	Width        float64  `json:"width"`
	Height       float64  `json:"height"`
	InViewport   bool     `json:"inViewport"`
	Visible      bool     `json:"visible"`
	PointerEvent string   `json:"pointerEvent"`
	Occluded     bool     `json:"occluded"`
	LocalX       float64  `json:"localX"`
	LocalY       float64  `json:"localY"`
	Top          occluder `json:"top"`
	// TransparentAncestor is kept apart from Visible because it is the one
	// hidden state the pointer itself can end: see revealTransparentAncestor.
	TransparentAncestor bool `json:"transparentAncestor"`
}

// occluder is the element the hit-test found instead of the target. Role and
// name come from the nearest ancestor-or-self that exposes either, because the
// element a pointer lands on is usually an anonymous div inside the overlay the
// agent actually sees.
type occluder struct {
	Tag  string `json:"tag"`
	Role string `json:"role"`
	Name string `json:"name"`
}

func (o occluder) String() string {
	label := strings.TrimSpace(o.Role)
	if o.Name != "" {
		label = strings.TrimSpace(label + " " + strconv.Quote(o.Name))
	}
	if label == "" {
		label = o.Tag
	}
	return label
}

// OccludedError carries the occluder's backend-node ancestry, hit element first,
// so a caller holding the tab's ref cache can name the overlay by the ref the
// agent already has.
type OccludedError struct {
	Top      occluder
	Ref      string
	Ancestry []int64
}

func (e *OccludedError) Error() string {
	msg := ErrElementOccluded.Error()
	if label := e.Top.String(); label != "" {
		msg += " by " + label
	}
	if e.Ref != "" {
		msg += " (" + e.Ref + ")"
	}
	return msg
}

func (e *OccludedError) Is(target error) bool { return target == ErrElementOccluded }

func (e *OccludedError) Describe(ref, role, name string) {
	e.Ref = ref
	if role != "" || name != "" {
		e.Top.Role, e.Top.Name = role, name
	}
}

const maxOccluderAncestry = 64

const elementAtLocalPointJS = `function(x, y) {
	const root = this.getRootNode();
	const doc = this.ownerDocument;
	return (typeof root.elementFromPoint === 'function' ? root : doc).elementFromPoint(x, y);
}`

const describeOccluderJS = `function(hit) {
	const implied = {a: 'link', button: 'button', dialog: 'dialog', nav: 'navigation', main: 'main',
		header: 'banner', footer: 'contentinfo', aside: 'complementary', form: 'form', img: 'img',
		select: 'combobox', textarea: 'textbox', h1: 'heading', h2: 'heading', h3: 'heading'};
	const text = (s) => String(s || '').replace(/\s+/g, ' ').trim().slice(0, 60);
	const attr = (el, n) => el.getAttribute ? text(el.getAttribute(n)) : '';
	const nameOf = (el) => {
		const ids = attr(el, 'aria-labelledby');
		const byId = ids ? ids.split(' ').map((id) => el.getRootNode().getElementById && el.getRootNode().getElementById(id)).filter(Boolean).map((n) => text(n.textContent)).join(' ') : '';
		return byId || attr(el, 'aria-label') || attr(el, 'title') || attr(el, 'alt') || '';
	};
	const tag = String(hit.tagName || '').toLowerCase();
	for (let el = hit; el && el.nodeType === 1; el = el.parentElement || (el.getRootNode().host || null)) {
		const t = String(el.tagName || '').toLowerCase();
		const role = attr(el, 'role') || (t === 'a' && !el.hasAttribute('href') ? '' : implied[t]) || (t === 'input' ? (attr(el, 'type') === 'checkbox' ? 'checkbox' : 'textbox') : '');
		let name = nameOf(el);
		if (!name && (role === 'button' || role === 'link' || role === 'heading')) name = text(el.textContent);
		if (role || name) return {tag, role, name};
	}
	return {tag, role: '', name: ''};
}`

const occluderObjectGroup = "pinchtab.occluder"

const occluderChainJS = `function(x, y, max) {
	const chain = [];
	for (let el = (` + elementAtLocalPointJS + `).call(this, x, y); el && chain.length < max; el = el.parentElement || (el.getRootNode().host || null)) {
		chain.push(el);
	}
	return chain;
}`

func occluderAncestry(ctx context.Context, objectID string, probe pointerProbe) []int64 {
	chainID, err := cdptk.CallFunctionOnForGroupedHandle(ctx, objectID, occluderChainJS,
		[]map[string]any{{"value": probe.LocalX}, {"value": probe.LocalY}, {"value": maxOccluderAncestry}}, occluderObjectGroup)
	if err != nil {
		return nil
	}
	defer releaseOccluderHandles(ctx)
	handles, err := cdptk.ArrayElementHandles(ctx, chainID)
	if err != nil {
		return nil
	}
	chain := make([]int64, 0, len(handles))
	for _, handle := range handles {
		nodeID, err := cdptk.BackendNodeIDForHandle(ctx, handle)
		if err != nil {
			break
		}
		chain = append(chain, nodeID)
	}
	return chain
}

func releaseOccluderHandles(ctx context.Context) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	_ = cdptk.ReleaseObjectGroup(ctx, occluderObjectGroup)
}

// transparentRevealWindow bounds how long a node hidden only by an opacity:0
// ancestor is given to become visible once the pointer is over it.
const transparentRevealWindow = 500 * time.Millisecond

const transparentRevealPoll = 50 * time.Millisecond

// PointerPointForNode validates clickability assumptions and returns a
// stable pointer coordinate for the backend node.
func PointerPointForNode(ctx context.Context, backendNodeID int64, requireTopMost bool) (float64, float64, error) {
	if err := cdptk.ScrollNodeIntoView(ctx, backendNodeID); err != nil {
		return 0, 0, fmt.Errorf("scroll into view: %w", err)
	}

	objectID, err := cdptk.IsolatedNodeObjectID(ctx, backendNodeID)
	if err != nil {
		return 0, 0, err
	}

	// view/doc come from the node, never from the ambient globals. In the main
	// world those globals were the node's own frame; in an isolated world the
	// handle carries no such guarantee, and reading them there would compute the
	// style, the frame walk, the viewport bounds and the occlusion hit-test
	// against the top frame for a node that lives in an iframe.
	const probeJS = `function() {
		const view = (this.ownerDocument && this.ownerDocument.defaultView) || window;
		const doc = this.ownerDocument;
		const r = this.getBoundingClientRect();
		const style = view.getComputedStyle(this);
		const localX = r.left + (r.width / 2);
		const localY = r.top + (r.height / 2);
		let x = localX;
		let y = localY;
		let topWindow = view;
		try {
			let current = view;
			while (current && current.parent && current !== current.parent) {
				const frameEl = current.frameElement;
				if (!frameEl) {
					break;
				}
				const frameRect = frameEl.getBoundingClientRect();
				x += frameRect.left;
				y += frameRect.top;
				current = current.parent;
				topWindow = current;
			}
		} catch (e) {
			// Cross-origin ancestors can block frame traversal. In that case we keep
			// the frame-local coordinates and let higher layers decide whether that
			// target is safely actionable.
		}
		const viewportWidth = topWindow && topWindow.innerWidth ? topWindow.innerWidth : view.innerWidth;
		const viewportHeight = topWindow && topWindow.innerHeight ? topWindow.innerHeight : view.innerHeight;
		const inViewport = x >= 0 && y >= 0 && x <= viewportWidth && y <= viewportHeight;
		// Opacity is the one of the three that does not reach a descendant on its
		// own. display:none gives the child no box at all, so the width/height
		// check below catches it; visibility:hidden is inherited, so the child's
		// own computed style already reports it. opacity:0 is neither — the child
		// keeps a real box and a computed opacity of 1 — so reading only this
		// element passed a button inside an opacity:0 panel as visible, and it
		// was clicked. An element nobody can see is the same hazard as the wrong
		// element: the caller is told OK about something it cannot perceive.
		//
		// Walks this document only, crossing shadow roots to their hosts. An
		// opacity:0 IFRAME hiding its contents is not covered here, because
		// reaching the frame owner can throw on a cross-origin ancestor and the
		// coordinate walk above already treats that as a case higher layers decide.
		const parentAcrossShadow = (n) => {
			if (n.parentElement) return n.parentElement;
			const root = n.getRootNode && n.getRootNode();
			return root && root.host ? root.host : null;
		};
		const transparentAncestor = () => {
			for (let n = this; n && n.nodeType === 1; n = parentAcrossShadow(n)) {
				const s = view.getComputedStyle(n);
				if (s && !(Number(s.opacity === '' || s.opacity == null ? '1' : s.opacity) > 0)) {
					return true;
				}
			}
			return false;
		};
		const visible = !!style && style.display !== 'none' && style.visibility !== 'hidden' && Number(style.opacity || '1') > 0;
		const pointerEvent = style ? String(style.pointerEvents || '') : '';
		let occluded = false;
		let top = {tag: '', role: '', name: ''};
		if (localX >= 0 && localY >= 0 && localX <= view.innerWidth && localY <= view.innerHeight) {
			const hit = (` + elementAtLocalPointJS + `).call(this, localX, localY);
			if (hit) {
				top = (` + describeOccluderJS + `)(hit);
				occluded = !(hit === this || this.contains(hit) || hit.contains(this));
			}
		}
		return {
			x,
			y,
			width: r.width,
			height: r.height,
			inViewport,
			visible,
			pointerEvent,
			occluded,
			localX,
			localY,
			top,
			transparentAncestor: transparentAncestor()
		};
	}`

	runProbe := func() (pointerProbe, error) {
		var probe pointerProbe
		if err := cdptk.CallFunctionOn(ctx, objectID, probeJS, nil, &probe); err != nil {
			return pointerProbe{}, fmt.Errorf("pointer probe: %w", err)
		}
		return probe, nil
	}

	probe, err := runProbe()
	if err != nil {
		return 0, 0, err
	}
	if probe.Visible && probe.TransparentAncestor && probe.Width > 0 && probe.Height > 0 {
		if probe, err = revealTransparentAncestor(ctx, probe, runProbe); err != nil {
			return 0, 0, err
		}
	}

	if probe.Width <= 0 || probe.Height <= 0 || !probe.Visible || probe.TransparentAncestor {
		return 0, 0, fmt.Errorf("%w: width=%.2f height=%.2f", ErrElementHidden, probe.Width, probe.Height)
	}
	if !probe.InViewport {
		return 0, 0, fmt.Errorf("%w: x=%.2f y=%.2f", ErrElementOffscreen, probe.X, probe.Y)
	}
	if strings.EqualFold(strings.TrimSpace(probe.PointerEvent), "none") {
		return 0, 0, fmt.Errorf("%w: pointer-events=none", ErrElementBlocked)
	}
	if requireTopMost && probe.Occluded {
		return 0, 0, &OccludedError{Top: probe.Top, Ancestry: occluderAncestry(ctx, objectID, probe)}
	}

	return probe.X, probe.Y, nil
}

// revealTransparentAncestor gives a node hidden only by an opacity:0 ancestor the
// chance a user's pointer would give it. Row actions revealed by
// .row:hover .actions{opacity:1}, and panels fading in from opacity:0, are both
// transparent at the instant of the first probe yet visible to anyone pointing
// at them. The pointer is moved to the point the action targets anyway, then the
// probe is repeated until the ancestor turns opaque or the window closes. Only a
// node that stays transparent under the pointer is refused.
func revealTransparentAncestor(ctx context.Context, probe pointerProbe, runProbe func() (pointerProbe, error)) (pointerProbe, error) {
	moveCtx, cancel := context.WithTimeout(ctx, transparentRevealWindow)
	_ = chromedp.Run(moveCtx, MouseEventAction(map[string]any{"type": "mouseMoved", "x": probe.X, "y": probe.Y}))
	cancel()

	deadline := time.Now().Add(transparentRevealWindow)
	for {
		next, err := runProbe()
		if err != nil {
			return probe, err
		}
		if !next.TransparentAncestor || time.Now().After(deadline) {
			return next, nil
		}
		select {
		case <-ctx.Done():
			return next, ctx.Err()
		case <-time.After(transparentRevealPoll):
		}
	}
}

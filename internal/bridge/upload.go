package bridge

import (
	"context"
	"fmt"
	"strings"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

func (b *Bridge) SetFileInputFiles(ctx context.Context, nodeID int64, paths []string) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return dom.SetFileInputFiles(paths).WithNodeID(cdp.NodeID(nodeID)).Do(ctx)
	}))
}

// ResolveSelectorToNodeID finds a DOM node by a unified selector string and returns its NodeID.
// Supports CSS (default), XPath (xpath: prefix or // auto-detect), and text (text: prefix).
func (b *Bridge) ResolveSelectorToNodeID(ctx context.Context, selector string) (int64, error) {
	var nodeID int64
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var expr string
		switch {
		case strings.HasPrefix(selector, "xpath:"):
			xpath := selector[len("xpath:"):]
			expr = fmt.Sprintf(`(function(){var r=document.evaluate(%q,document,null,XPathResult.FIRST_ORDERED_NODE_TYPE,null);return r.singleNodeValue})()`, xpath)
		case strings.HasPrefix(selector, "//") || strings.HasPrefix(selector, "(//"):
			expr = fmt.Sprintf(`(function(){var r=document.evaluate(%q,document,null,XPathResult.FIRST_ORDERED_NODE_TYPE,null);return r.singleNodeValue})()`, selector)
		case strings.HasPrefix(selector, "text:"):
			text := selector[len("text:"):]
			expr = fmt.Sprintf(`(function(){var w=document.createTreeWalker(document.body,NodeFilter.SHOW_TEXT);while(w.nextNode()){if(w.currentNode.textContent.includes(%q))return w.currentNode.parentElement}return null})()`, text)
		case strings.HasPrefix(selector, "css:"):
			css := selector[len("css:"):]
			expr = fmt.Sprintf(`document.querySelector(%q)`, css)
		default:
			// Bare selector — treat as CSS (backward compatible)
			expr = fmt.Sprintf(`document.querySelector(%q)`, selector)
		}

		val, _, err := runtime.Evaluate(expr).Do(ctx)
		if err != nil {
			return fmt.Errorf("evaluate: %w", err)
		}
		if val.ObjectID == "" {
			return fmt.Errorf("no element matches selector")
		}
		node, err := dom.RequestNode(val.ObjectID).Do(ctx)
		if err != nil {
			return fmt.Errorf("request node: %w", err)
		}
		nodeID = int64(node)
		return nil
	}))
	return nodeID, err
}

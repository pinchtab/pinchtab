package bridge

import (
	"context"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

func (b *Bridge) Evaluate(ctx context.Context, expression string, result any, opts EvalOpts) error {
	var chromedpOpts []chromedp.EvaluateOption
	if opts.AwaitPromise {
		chromedpOpts = append(chromedpOpts, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
			return p.WithAwaitPromise(true)
		})
	}
	return chromedp.Run(ctx, chromedp.Evaluate(expression, result, chromedpOpts...))
}

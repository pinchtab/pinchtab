package extract

import (
	"context"

	"github.com/pinchtab/pinchtab/internal/bridge/observe"
)

func Resolve(schema Schema, nodes []observe.A11yNode, opts Options) Result {
	return ResolveContext(context.Background(), schema, nodes, opts)
}

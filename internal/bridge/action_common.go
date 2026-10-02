package bridge

import "context"

func targetNodeID(ctx context.Context, nodeID int64, css string) (int64, error) {
	if nodeID > 0 || css == "" {
		return nodeID, nil
	}
	return ResolveCSSToNodeID(ctx, css)
}

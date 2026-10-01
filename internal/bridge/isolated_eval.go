package bridge

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pinchtab/pinchtab/internal/cdptk"
)

// BackendNodeIDForSelector resolves a CSS selector to a backend node id in the
// isolated world of frameID (top frame when empty). It returns 0 without error
// when the selector matches nothing, so a caller mapping many selectors treats a
// miss as "no ref" rather than a failure that aborts the whole mapping.
func BackendNodeIDForSelector(ctx context.Context, frameID, selector string) (int64, error) {
	execID, err := cdptk.IsolatedContextID(ctx, frameID)
	if err != nil {
		return 0, err
	}

	encodedSelector, err := json.Marshal(selector)
	if err != nil {
		return 0, err
	}

	objectID, err := cdptk.EvaluateForHandle(ctx, execID, fmt.Sprintf(`document.querySelector(%s)`, encodedSelector))
	if err != nil {
		return 0, err
	}
	if objectID == "" {
		return 0, nil
	}
	return backendNodeIDFromObjectID(ctx, objectID)
}

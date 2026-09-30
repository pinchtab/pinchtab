package bridge

import (
	"context"

	bridgetabs "github.com/pinchtab/pinchtab/internal/bridge/tabs"
)

const DefaultLockTimeout = bridgetabs.DefaultLockTimeout

type DialogState = bridgetabs.DialogState

func HandlePendingDialog(ctx context.Context, tabID string, dm *bridgetabs.DialogManager, accept bool, promptText string) (*bridgetabs.DialogResult, error) {
	return bridgetabs.HandlePendingDialog(ctx, tabID, dm, accept, promptText)
}

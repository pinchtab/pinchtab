package bridge

import (
	"context"

	bridgetabs "github.com/pinchtab/pinchtab/internal/bridge/tabs"
)

const DefaultLockTimeout = bridgetabs.DefaultLockTimeout

type DialogState = bridgetabs.DialogState
type DialogManager = bridgetabs.DialogManager
type DialogResult = bridgetabs.DialogResult
type LockManager = bridgetabs.LockManager
type TabExecutor = bridgetabs.TabExecutor
type ExecutorStats = bridgetabs.ExecutorStats

func HandlePendingDialog(ctx context.Context, tabID string, dm *DialogManager, accept bool, promptText string) (*DialogResult, error) {
	return bridgetabs.HandlePendingDialog(ctx, tabID, dm, accept, promptText)
}

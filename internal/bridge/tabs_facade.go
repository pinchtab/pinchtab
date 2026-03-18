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

func NewDialogManager() *DialogManager {
	return bridgetabs.NewDialogManager()
}

func HandleDialog(ctx context.Context, accept bool, promptText string) error {
	return bridgetabs.HandleDialog(ctx, accept, promptText)
}

func ListenDialogEvents(ctx context.Context, tabID string, dm *DialogManager, autoAccept bool) {
	bridgetabs.ListenDialogEvents(ctx, tabID, dm, autoAccept)
}

func EnableDialogEvents(ctx context.Context) error {
	return bridgetabs.EnableDialogEvents(ctx)
}

func HandlePendingDialog(ctx context.Context, tabID string, dm *DialogManager, accept bool, promptText string) (*DialogResult, error) {
	return bridgetabs.HandlePendingDialog(ctx, tabID, dm, accept, promptText)
}

func NewLockManager() *LockManager {
	return bridgetabs.NewLockManager()
}

func NewTabExecutor(maxParallel int) *TabExecutor {
	return bridgetabs.NewTabExecutor(maxParallel)
}

func DefaultMaxParallel() int {
	return bridgetabs.DefaultMaxParallel()
}

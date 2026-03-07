package scheduler

import (
	"fmt"

	"github.com/pinchtab/pinchtab/internal/instance"
)

// ManagerResolver adapts instance.Manager to the InstanceResolver interface.
type ManagerResolver struct {
	Mgr *instance.Manager
}

func (r *ManagerResolver) ResolveTabInstance(tabID string) (string, error) {
	inst, err := r.Mgr.FindInstanceByTabID(tabID)
	if err != nil {
		return "", fmt.Errorf("tab %q not found: %w", tabID, err)
	}
	return inst.Port, nil
}

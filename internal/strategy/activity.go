package strategy

import (
	"net/http"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/orchestrator"
)

func EnrichForTarget(r *http.Request, orch *orchestrator.Orchestrator, target string) {
	if r == nil || orch == nil || target == "" {
		return
	}

	for _, inst := range orch.List() {
		if inst.Status != "running" || inst.URL != target {
			continue
		}
		activity.EnrichRequest(r, activity.Update{
			InstanceID:  inst.ID,
			ProfileID:   inst.ProfileID,
			ProfileName: inst.ProfileName,
		})
		return
	}
}

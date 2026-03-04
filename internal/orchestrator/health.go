package orchestrator

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

const (
	instanceHealthPollInterval = 500 * time.Millisecond
	instanceStartupTimeout     = 45 * time.Second
)

func (o *Orchestrator) monitor(inst *InstanceInternal) {
	healthy := false
	exitedEarly := false
	lastProbe := "no response"
	resolvedURL := ""
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- inst.cmd.Wait()
	}()
	var waitErr error
	started := time.Now()
	for time.Since(started) < instanceStartupTimeout {
		select {
		case waitErr = <-waitCh:
			exitedEarly = true
		default:
		}
		if exitedEarly {
			break
		}
		time.Sleep(instanceHealthPollInterval)

		for _, baseURL := range instanceBaseURLs(inst.Port) {
			// Suppress CodeQL alert: baseURL comes from trusted orchestrator configuration
			// (port-based child instance list), not user input. This is intentional design:
			// agents request the orchestrator to probe known child instances.
			// lgtm[go/request-forgery]
			req, reqErr := http.NewRequest(http.MethodGet, baseURL+"/health", nil)
			if reqErr != nil {
				lastProbe = fmt.Sprintf("%s -> %s", baseURL, reqErr.Error())
				continue
			}
			if o.childAuthToken != "" {
				req.Header.Set("Authorization", "Bearer "+o.childAuthToken)
			}
			resp, err := o.client.Do(req)
			if err == nil {
				_ = resp.Body.Close()
				lastProbe = fmt.Sprintf("%s -> HTTP %d", baseURL, resp.StatusCode)
				if isInstanceHealthyStatus(resp.StatusCode) {
					healthy = true
					resolvedURL = baseURL
					break
				}
			} else {
				lastProbe = fmt.Sprintf("%s -> %s", baseURL, err.Error())
			}
		}
		if healthy {
			break
		}
	}

	o.mu.Lock()
	var eventType string
	switch inst.Status {
	case "stopping", "stopped":
	default:
		if healthy {
			inst.Status = "running"
			if resolvedURL != "" {
				inst.URL = resolvedURL
			}
			eventType = "instance.started"
			slog.Info("instance ready", "id", inst.ID, "port", inst.Port)
		} else if exitedEarly {
			inst.Status = "error"
			if waitErr != nil {
				inst.Error = "process exited before health check: " + waitErr.Error()
			} else {
				inst.Error = "process exited before health check succeeded"
			}
			if tail := tailLogLine(inst.logBuf.String()); tail != "" {
				inst.Error += " | " + tail
			}
			eventType = "instance.error"
			slog.Error("instance exited before ready", "id", inst.ID)
		} else {
			inst.Status = "error"
			inst.Error = fmt.Errorf("health check timeout after %s (%s)", instanceStartupTimeout, lastProbe).Error()
			if tail := tailLogLine(inst.logBuf.String()); tail != "" {
				inst.Error += " | " + tail
			}
			eventType = "instance.error"
			slog.Error("instance failed to start", "id", inst.ID)
		}
	}
	instCopy := inst.Instance
	o.mu.Unlock()
	if eventType != "" {
		o.emitEvent(eventType, &instCopy)
	}

	if !exitedEarly {
		<-waitCh
	}

	o.mu.Lock()
	wasRunning := inst.Status == "running"
	wasStopping := inst.Status == "stopping"

	// Reset restart counter if the instance ran long enough to be considered stable.
	if inst.restartCount > 0 && !inst.lastCrash.IsZero() && time.Since(inst.lastCrash) > o.stableAfter {
		slog.Info("instance was stable, resetting restart counter",
			"id", inst.ID,
			"uptime", time.Since(inst.lastCrash).Round(time.Second),
		)
		inst.restartCount = 0
	}

	// Auto-restart: if instance crashed while running (not a deliberate stop)
	// and we haven't exceeded the restart limit.
	shouldRestart := o.autoRestart && wasRunning && !wasStopping && inst.restartCount < o.maxRestarts

	if shouldRestart {
		inst.restartCount++
		inst.lastCrash = time.Now()
		inst.Status = "restarting"
		instCopy = inst.Instance
		backoff := o.restartBackoff * time.Duration(1<<uint(inst.restartCount-1))
		o.mu.Unlock()

		slog.Warn("instance crashed, scheduling auto-restart",
			"id", inst.ID,
			"attempt", inst.restartCount,
			"maxRestarts", o.maxRestarts,
			"backoff", backoff,
		)
		o.emitEvent("instance.restarting", &instCopy)
		time.Sleep(backoff)
		o.restartInstance(inst)
		return
	}

	// No restart — mark stopped as before.
	wasStopped := false
	if wasRunning || wasStopping {
		inst.Status = "stopped"
		wasStopped = true
	}
	instCopy = inst.Instance
	o.mu.Unlock()
	if wasStopped {
		o.emitEvent("instance.stopped", &instCopy)
	}
	slog.Info("instance exited", "id", inst.ID)
}

type remoteTab struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Title string `json:"title"`
}

type remoteMetrics struct {
	Memory *memoryMetrics `json:"memory,omitempty"`
}

type memoryMetrics struct {
	JSHeapUsedMB  float64 `json:"jsHeapUsedMB"`
	JSHeapTotalMB float64 `json:"jsHeapTotalMB"`
	Documents     int64   `json:"documents"`
	Frames        int64   `json:"frames"`
	Nodes         int64   `json:"nodes"`
	Listeners     int64   `json:"listeners"`
}

func (o *Orchestrator) fetchTabs(baseURL string) ([]remoteTab, error) {
	req, err := http.NewRequest(http.MethodGet, baseURL+"/screencast/tabs", nil)
	if err != nil {
		return nil, err
	}
	if o.childAuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+o.childAuthToken)
	}

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var tabs []remoteTab
	if err := json.NewDecoder(resp.Body).Decode(&tabs); err != nil {
		return nil, err
	}
	return tabs, nil
}

func (o *Orchestrator) fetchMetrics(baseURL string) (*memoryMetrics, error) {
	req, err := http.NewRequest(http.MethodGet, baseURL+"/metrics", nil)
	if err != nil {
		return nil, err
	}
	if o.childAuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+o.childAuthToken)
	}

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return nil, nil
	}

	var result remoteMetrics
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Memory, nil
}

func isInstanceHealthyStatus(code int) bool {
	return code > 0 && code < http.StatusInternalServerError
}

func instanceBaseURLs(port string) []string {
	return []string{
		fmt.Sprintf("http://127.0.0.1:%s", port),
		fmt.Sprintf("http://[::1]:%s", port),
		fmt.Sprintf("http://localhost:%s", port),
	}
}

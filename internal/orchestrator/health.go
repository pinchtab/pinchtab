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
			resp, err := o.client.Get(baseURL + "/health")
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
	switch inst.Status {
	case "stopping", "stopped":
	default:
		if healthy {
			inst.Status = "running"
			if resolvedURL != "" {
				inst.URL = resolvedURL
			}
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
			slog.Error("instance exited before ready", "id", inst.ID)
		} else {
			inst.Status = "error"
			inst.Error = fmt.Errorf("health check timeout after %s (%s)", instanceStartupTimeout, lastProbe).Error()
			if tail := tailLogLine(inst.logBuf.String()); tail != "" {
				inst.Error += " | " + tail
			}
			slog.Error("instance failed to start", "id", inst.ID)
		}
	}
	o.mu.Unlock()

	if !exitedEarly {
		<-waitCh
	}
	o.mu.Lock()
	if inst.Status == "running" || inst.Status == "stopping" {
		inst.Status = "stopped"
	}
	o.mu.Unlock()
	slog.Info("instance exited", "id", inst.ID)
}

type remoteTab struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

func (o *Orchestrator) fetchTabs(baseURL string) ([]remoteTab, error) {
	resp, err := o.client.Get(baseURL + "/screencast/tabs")
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

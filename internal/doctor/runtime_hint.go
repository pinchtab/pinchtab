package doctor

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
)

// ScopeStatement is printed with every run: doctor inspects the installation on
// disk and never the running system, which is what lets it work when the browser
// will not start and there is no server to ask.
const ScopeStatement = "Scope: the installation on disk, not any running PinchTab server or browser. " +
	"Runtime state lives at GET /health (crashes, security), `pinchtab security` and GET /instances/{id}/metrics."

const runtimeProbeTimeout = time.Second

// ProbeRuntime names a server answering at the configured address, best-effort.
// It gates nothing and changes no result: an empty answer is the normal case.
func ProbeRuntime(ctx context.Context, cfg *config.RuntimeConfig) string {
	if cfg == nil || strings.TrimSpace(cfg.Port) == "" {
		return ""
	}
	host := strings.TrimSpace(cfg.Bind)
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	base := "http://" + net.JoinHostPort(host, strings.TrimSpace(cfg.Port))
	ctx, cancel := context.WithTimeout(ctx, runtimeProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/health", nil)
	if err != nil {
		return ""
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	_ = resp.Body.Close()
	return fmt.Sprintf("A PinchTab server is answering at %s: read %s/health for the running browser's crashes and security posture.", base, base)
}

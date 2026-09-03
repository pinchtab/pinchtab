package server

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/bridgeregistry"
	_ "github.com/pinchtab/pinchtab/internal/browsers/all"
	"github.com/pinchtab/pinchtab/internal/browsers/providerhooks"
	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/handlers"
)

func RunBridgeServer(cfg *config.RuntimeConfig, version string) {
	listenAddr := cfg.ListenAddr()
	cli.PrintStartupBanner(cfg, cli.StartupBannerOptions{
		Mode:         "bridge",
		ListenAddr:   listenAddr,
		ListenStatus: "starting",
		ProfileDir:   cfg.ProfileDir,
	})

	providerhooks.CleanupProfile(config.NormalizeBrowser(cfg.DefaultBrowser), cfg.ProfileDir)

	bridgeInstance := bridge.New(context.Background(), nil, cfg)
	actStore, err := activity.NewRecorder(activity.Config{
		Enabled:       cfg.Observability.Activity.Enabled,
		RetentionDays: cfg.Observability.Activity.RetentionDays,
		Events: activity.EventSourceConfig{
			Dashboard:    cfg.Observability.Activity.Events.Dashboard,
			Server:       cfg.Observability.Activity.Events.Server,
			Bridge:       cfg.Observability.Activity.Events.Bridge,
			Orchestrator: cfg.Observability.Activity.Events.Orchestrator,
			Scheduler:    cfg.Observability.Activity.Events.Scheduler,
			MCP:          cfg.Observability.Activity.Events.MCP,
			Other:        cfg.Observability.Activity.Events.Other,
		},
	}, cfg.ActivityLogDir())
	if err != nil {
		fatalStartup("activity store", err)
	}

	mux := http.NewServeMux()
	h := handlers.New(bridgeInstance, cfg, nil, nil, nil)
	h.Version = version
	h.StartBackgroundCleanup()
	configureBridgeRouter(h, cfg)

	var server *http.Server
	shutdownOnce := &sync.Once{}
	doShutdown := func() {
		shutdownOnce.Do(func() {
			slog.Info("shutting down bridge...")
			if bridgeInstance != nil {
				bridgeInstance.Cleanup()
			}
			// POST /shutdown only reaches this closure, not the SIGINT/SIGTERM
			// select below — without an explicit Shutdown here the listener
			// keeps serving (including /health) forever, so a caller polling
			// for the process to actually stop never observes it exit.
			if server != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := server.Shutdown(ctx); err != nil {
					slog.Error("shutdown http", "err", err)
				}
			}
		})
	}
	h.RegisterRoutes(mux, doShutdown)
	activity.RegisterHandlers(mux, actStore)
	RegisterSessionsUnavailableInBridgeMode(mux)
	cli.LogSecurityWarnings(cfg)

	// A bridge process registers no ConfigAPI and no orchestrator, so nothing
	// here ever publishes a second value: this Live holds the boot config for the
	// life of the process by construction, not by accident. The middlewares still
	// resolve per request, which is what keeps one contract for both processes.
	live := config.NewLive(cfg)

	server = &http.Server{
		Addr: listenAddr,
		Handler: handlers.TrustedInternalProxyStripMiddleware(os.Getenv("PINCHTAB_INTERNAL_TOKEN"))(
			handlers.RequestIDMiddleware(
				activity.Middleware(
					actStore,
					"bridge",
					handlers.SecurityHeadersMiddleware(live,
						handlers.LoggingMiddleware(handlers.RateLimitMiddleware(handlers.AuthMiddleware(live, mux))),
					),
				),
			),
		),
		MaxHeaderBytes:    maxHeaderBytes,
		ReadHeaderTimeout: serverReadHeaderTimeout,
		ReadTimeout:       serverReadTimeout,
		WriteTimeout:      serverWriteTimeout,
		IdleTimeout:       serverIdleTimeout,
	}

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		fatalStartup("cannot listen on "+listenAddr, err)
	}
	applyBoundBridgePort(cfg, listener.Addr())
	registration, err := registerBridge(cfg, listener.Addr())
	if err != nil {
		_ = listener.Close()
		fatalStartup("bridge registry", err)
	}
	defer func() {
		if err := registration.Close(); err != nil {
			slog.Warn("remove bridge registry record", "err", err)
		}
	}()

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)
	select {
	case <-sigChan:
	case err := <-serveErr:
		doShutdown()
		if err != nil && err != http.ErrServerClosed {
			_ = registration.Close()
			fatalStartup("server error", err)
		}
		return
	}

	doShutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		slog.Error("shutdown error", "err", err)
	}
}

func applyBoundBridgePort(cfg *config.RuntimeConfig, listenerAddr net.Addr) {
	if cfg == nil || listenerAddr == nil {
		return
	}
	_, port, err := net.SplitHostPort(listenerAddr.String())
	if err == nil && strings.TrimSpace(port) != "" {
		cfg.Port = port
	}
}

func registerBridge(cfg *config.RuntimeConfig, listenerAddr net.Addr) (*bridgeregistry.Registration, error) {
	cdpIdentity := strings.TrimSpace(cfg.CDPAttachURL)
	if cdpIdentity == "" {
		cdpIdentity = strings.TrimSpace(cfg.RemoteCDPURL)
	}
	browserType := strings.TrimSpace(cfg.DefaultBrowser)
	if browserType == "" {
		browserType = config.BrowserChrome
	}
	address := strings.TrimSpace(cfg.Bind)
	port := strings.TrimSpace(cfg.Port)
	if listenerAddr != nil {
		if listenerHost, listenerPort, err := net.SplitHostPort(listenerAddr.String()); err == nil {
			port = listenerPort
			if address == "" {
				address = listenerHost
			}
		}
	}
	return bridgeregistry.Register(cfg.StateDir, bridgeregistry.Record{
		Address:      address,
		Port:         port,
		CDPIdentity:  cdpIdentity,
		BrowserType:  browserType,
		BrowserLabel: cfg.RemoteBrowserName,
	})
}

func configureBridgeRouter(h *handlers.Handlers, cfg *config.RuntimeConfig) {
	decorated := providerhooks.DecorateBridge(config.NormalizeBrowser(cfg.DefaultBrowser), h.Bridge, cfg)
	if decorated == h.Bridge {
		return
	}
	h.Bridge = decorated
	slog.Info("browser bridge proxy enabled", "browser", config.NormalizeBrowser(cfg.DefaultBrowser))
}

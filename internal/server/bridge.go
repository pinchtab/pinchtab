package server

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
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
	}, cfg.ActivityStateDir())
	if err != nil {
		slog.Error("activity store", "err", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	h := handlers.New(bridgeInstance, cfg, nil, nil, nil)
	h.Version = version
	h.StartBackgroundCleanup()
	configureBridgeRouter(h, cfg)

	shutdownOnce := &sync.Once{}
	doShutdown := func() {
		shutdownOnce.Do(func() {
			slog.Info("shutting down bridge...")
			if bridgeInstance != nil {
				bridgeInstance.Cleanup()
			}
		})
	}
	h.RegisterRoutes(mux, doShutdown)
	activity.RegisterHandlers(mux, actStore)
	cli.LogSecurityWarnings(cfg)

	server := &http.Server{
		Addr: listenAddr,
		Handler: handlers.TrustedInternalProxyStripMiddleware(os.Getenv("PINCHTAB_INTERNAL_TOKEN"))(
			handlers.RequestIDMiddleware(
				activity.Middleware(
					actStore,
					"bridge",
					handlers.SecurityHeadersMiddleware(cfg,
						handlers.LoggingMiddleware(handlers.RateLimitMiddleware(handlers.AuthMiddleware(cfg, mux))),
					),
				),
			),
		),
		MaxHeaderBytes:    maxHeaderBytes,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	doShutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		slog.Error("shutdown error", "err", err)
	}
}

func configureBridgeRouter(h *handlers.Handlers, cfg *config.RuntimeConfig) {
	decorated := providerhooks.DecorateBridge(config.NormalizeBrowser(cfg.DefaultBrowser), h.Bridge, cfg)
	if decorated == h.Bridge {
		return
	}
	h.Bridge = decorated
	slog.Info("browser bridge proxy enabled", "browser", config.NormalizeBrowser(cfg.DefaultBrowser))
}

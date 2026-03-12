package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/pinchtab/pinchtab/internal/assets"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/engine"
	"github.com/pinchtab/pinchtab/internal/handlers"
)

// runBridgeServer starts a bridge without orchestrator or dashboard
// This is used for spawned instances by the orchestrator
func runBridgeServer(cfg *config.RuntimeConfig) {
	listenAddr := cfg.ListenAddr()
	printStartupBanner(cfg, startupBannerOptions{
		Mode:       "bridge",
		ListenAddr: listenAddr,
		ProfileDir: cfg.ProfileDir,
	})

	// Create a bridge instance with lazy initialization
	// Chrome will be initialized on first request via ensureChrome()
	bridgeInstance := bridge.New(context.Background(), nil, cfg)
	bridgeInstance.StealthScript = assets.StealthScript

	mux := http.NewServeMux()

	// Register all bridge handlers
	h := handlers.New(bridgeInstance, cfg, nil, nil, nil)

	// Wire engine router for alternative engine modes.
	mode := engine.Mode(cfg.Engine)
	switch mode {
	case engine.ModeLite, engine.ModeAuto:
		lite := engine.NewLiteEngine()
		h.Router = engine.NewRouter(mode, lite)
		slog.Info("engine router enabled", "mode", cfg.Engine, "rules", h.Router.Rules())
	case engine.ModeLightpanda:
		lpURL := cfg.LightpandaURL
		if lpURL == "" {
			lpURL = "ws://127.0.0.1:9222"
		}
		lp, err := engine.NewLightpandaEngine(lpURL)
		if err != nil {
			slog.Error("failed to create lightpanda engine", "err", err)
			os.Exit(1)
		}
		h.Router = engine.NewRouter(mode, nil)
		h.Router.RegisterEngine(lp)
		slog.Info("engine router enabled", "mode", cfg.Engine, "url", lpURL, "rules", h.Router.Rules())
	}

	shutdownOnce := &sync.Once{}
	doShutdown := func() {
		shutdownOnce.Do(func() {
			slog.Info("shutting down bridge...")
		})
	}
	h.RegisterRoutes(mux, doShutdown)
	logSecurityWarnings(cfg)

	// HTTP server
	server := &http.Server{
		Addr:              listenAddr,
		Handler:           handlers.RequestIDMiddleware(handlers.LoggingMiddleware(handlers.AuthMiddleware(cfg, mux))),
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

	// Graceful shutdown on signal
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

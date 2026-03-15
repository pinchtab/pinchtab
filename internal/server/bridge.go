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

	"github.com/pinchtab/pinchtab/internal/assets"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/engine"
	"github.com/pinchtab/pinchtab/internal/handlers"
)

func RunBridgeServer(cfg *config.RuntimeConfig) {
	listenAddr := cfg.ListenAddr()
	cli.PrintStartupBanner(cfg, cli.StartupBannerOptions{
		Mode:         "bridge",
		ListenAddr:   listenAddr,
		ListenStatus: "starting",
		ProfileDir:   cfg.ProfileDir,
	})

	// Clean up orphaned Chrome processes from previous crashed runs
	bridge.CleanupOrphanedChromeProcesses(cfg.ProfileDir)

	bridgeInstance := bridge.New(context.Background(), nil, cfg)
	bridgeInstance.StealthScript = assets.StealthScript

	mux := http.NewServeMux()
	h := handlers.New(bridgeInstance, cfg, nil, nil, nil)
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
	cli.LogSecurityWarnings(cfg)

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
	mode := engine.Mode(cfg.Engine)
	if mode != engine.ModeLite && mode != engine.ModeAuto {
		return
	}

	lite := engine.NewLiteEngine()
	h.Router = engine.NewRouter(mode, lite)
	slog.Info("engine router enabled", "mode", cfg.Engine, "rules", h.Router.Rules())
}

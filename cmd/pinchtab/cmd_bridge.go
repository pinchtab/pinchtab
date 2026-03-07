package main

import (
	"context"
	"fmt"
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
	"github.com/pinchtab/pinchtab/internal/handlers"
)

// runBridgeServer starts a bridge server without orchestrator or dashboard
// This is used for spawned instances by the orchestrator
func runBridgeServer(cfg *config.RuntimeConfig) {
	listenAddr := cfg.ListenAddr()
	slog.Info("🦀 Pinchtab Bridge Server", "listen", listenAddr, "profile", cfg.ProfileDir)

	// Create a bridge instance with lazy initialization
	// Chrome will be initialized on first request via ensureChrome()
	bridgeInstance := bridge.New(context.Background(), nil, cfg)
	bridgeInstance.StealthScript = assets.StealthScript

	mux := http.NewServeMux()

	// Register all bridge handlers
	h := handlers.New(bridgeInstance, cfg, nil, nil, nil)
	shutdownOnce := &sync.Once{}
	doShutdown := func() {
		shutdownOnce.Do(func() {
			slog.Info("shutting down bridge server...")
		})
	}
	h.RegisterRoutes(mux, doShutdown)
	if cfg.AllowEvaluate && cfg.Token == "" {
		slog.Warn("evaluate endpoint enabled without API token", "hint", "set PINCHTAB_TOKEN for authenticated access")
	}

	// HTTP server
	server := &http.Server{
		Addr:              listenAddr,
		Handler:           handlers.RequestIDMiddleware(handlers.LoggingMiddleware(recoveryMiddleware(handlers.AuthMiddleware(cfg, mux)))),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		slog.Info("bridge server listening", "addr", listenAddr)
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

// recoveryMiddleware catches panics in HTTP handlers and returns a 500
// instead of crashing the bridge process. Go's net/http server only
// recovers panics in the serve goroutine; this middleware provides the
// same guarantee for the handler level and logs the panic for debugging.
func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				handlers.RecordRecoveredPanic()
				slog.Error("handler panic recovered",
					"requestId", w.Header().Get("X-Request-Id"),
					"method", r.Method,
					"path", r.URL.Path,
					"panic", fmt.Sprintf("%v", p),
				)
				http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

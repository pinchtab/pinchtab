package main

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
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
	// Prepend seed + stealth level so the 'full' block in stealth.js actually executes.
	// Without this, __pinchtab_stealth_level defaults to 'light' and advanced stealth
	// (screen override, WebGL, canvas noise, etc.) is dead code.
	bridgeInstance.StealthScript = fmt.Sprintf(
		"var __pinchtab_seed = %d;\nvar __pinchtab_stealth_level = %q;\n",
		rand.Intn(1000000000), cfg.StealthLevel,
	) + assets.StealthScript

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

	// HTTP server
	server := &http.Server{
		Addr:              listenAddr,
		Handler:           loggingMiddleware(mux),
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

// Simple middleware to log HTTP requests
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, statusCode: 200}

		next.ServeHTTP(recorder, r)

		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.statusCode,
			"ms", time.Since(start).Milliseconds(),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

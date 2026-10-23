package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/clover-eric/dcst/internal/dcst"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	dataDir := envString("DATA_DIR", "/data")
	enginePath := envString("CFST_PATH", "/app/cfst")
	listenAddr := envString("LISTEN_ADDR", ":10123")
	app, err := dcst.NewApp(dataDir, enginePath, logger)
	if err != nil {
		logger.Error("初始化失败", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go app.Scheduler(ctx)

	server := &http.Server{
		Addr:              listenAddr,
		Handler:           dcst.NewHandler(app),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	logger.Info("DCST 已启动", "listen", listenAddr, "version", dcst.BuildVersion)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("HTTP 服务退出", "error", err)
		os.Exit(1)
	}
}

func envString(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

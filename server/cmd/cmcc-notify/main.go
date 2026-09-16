package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/thekfjie/cmcc-notify/cmcc"
	"github.com/thekfjie/cmcc-notify/server/internal/config"
	"github.com/thekfjie/cmcc-notify/server/internal/httpapi"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "probe" {
		if err := probe(); err != nil {
			slog.Error("CMCC probe failed", "error", err)
			os.Exit(1)
		}
		fmt.Println("CMCC authentication succeeded")
		return
	}
	configPath := flag.String("config", "config.yaml", "configuration file")
	flag.Parse()
	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}
	api, err := httpapi.New(cfg, version)
	if err != nil {
		slog.Error("create server", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := api.Start(ctx); err != nil {
		slog.Error("start CMCC clients", "error", err)
		os.Exit(1)
	}
	httpServer := &http.Server{
		Addr:              cfg.Listen,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       60 * time.Second,
	}
	serverErr := make(chan error, 1)
	go func() { serverErr <- httpServer.ListenAndServe() }()
	slog.Info("cmcc-notify server started", "version", version, "listen", cfg.Listen, "accounts", cfg.Redacted())
	select {
	case <-ctx.Done():
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server stopped", "error", err)
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
	api.Close()
}

func probe() error {
	key := os.Getenv("CMCC_API_KEY")
	if key == "" {
		return errors.New("CMCC_API_KEY is required")
	}
	client, err := cmcc.NewClient(key, cmcc.Config{ServerURL: os.Getenv("CMCC_SERVER_URL")})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := client.Connect(ctx); err != nil {
		return err
	}
	return client.Close()
}

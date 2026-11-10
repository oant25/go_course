package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/oant25/go_course/internal/config"
	"github.com/oant25/go_course/internal/container"
	"github.com/oant25/go_course/internal/diagnostic"
	"github.com/oant25/go_course/internal/infra/telemetry"
)

func main() {
	if err := run(); err != nil {
		logger := telemetry.NewLogger(os.Stdout, os.Getenv("OTEL_SERVICE_NAME"), slog.LevelInfo)
		message := "service stopped with error"
		if errors.Is(err, context.DeadlineExceeded) {
			message = "service deadline exceeded; forcing exit"
		}
		var configErr *config.Error
		if errors.As(err, &configErr) {
			logger.Error("invalid service configuration", "error", configErr.Error())
		} else {
			logger.LogAttrs(context.Background(), slog.LevelError, message, diagnostic.Attributes(err)...)
		}
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	app, err := container.New(ctx, cfg)
	if err != nil {
		return fmt.Errorf("initialize service: %w", err)
	}
	return app.Run(ctx)
}

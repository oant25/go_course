package container

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/oant25/go_course/internal/infra/telemetry"
)

type App struct {
	telemetry       *telemetry.Runtime
	server          *http.Server
	pool            *pgxpool.Pool
	logger          *slog.Logger
	shutdownTimeout time.Duration
}

func (a *App) Run(ctx context.Context) (runErr error) {
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), a.shutdownTimeout)
		defer cancel()
		a.logger.InfoContext(shutdownCtx, "service stopping")
		runErr = errors.Join(runErr, a.shutdown(shutdownCtx))
		if runErr == nil {
			a.logger.InfoContext(shutdownCtx, "service stopped")
		}
	}()
	var listenerConfig net.ListenConfig
	listener, err := listenerConfig.Listen(ctx, "tcp", a.server.Addr)
	if err != nil {
		return fmt.Errorf("listen HTTP: %w", err)
	}
	a.logger.InfoContext(ctx, "service started", "address", listener.Addr().String())
	stopped := make(chan error, 1)
	go func() { stopped <- a.server.Serve(listener) }()
	select {
	case <-ctx.Done():
		return nil
	case err := <-stopped:
		if errors.Is(err, http.ErrServerClosed) && ctx.Err() != nil {
			return nil
		}
		if err == nil {
			return errors.New("HTTP server stopped unexpectedly")
		}
		return fmt.Errorf("serve HTTP: %w", err)
	}
}

func (a *App) shutdown(ctx context.Context) error {
	var result error
	if err := a.server.Shutdown(ctx); err != nil {
		result = fmt.Errorf("shutdown HTTP: %w", err)
		if closeErr := a.server.Close(); closeErr != nil {
			result = errors.Join(result, fmt.Errorf("close HTTP: %w", closeErr))
		}
	}
	if err := a.telemetry.Shutdown(ctx); err != nil {
		result = errors.Join(result, fmt.Errorf("shutdown telemetry: %w", err))
	}
	if err := closePool(ctx, a.pool); err != nil {
		result = errors.Join(result, err)
	}
	return result
}

func closePool(ctx context.Context, pool *pgxpool.Pool) error {
	closed := make(chan struct{})
	go func() { pool.Close(); close(closed) }()
	select {
	case <-closed:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("close PostgreSQL pool: %w", ctx.Err())
	}
}

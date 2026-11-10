package container

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/oant25/go_course/internal/config"
	transport "github.com/oant25/go_course/internal/http"
	"github.com/oant25/go_course/internal/http/health"
	"github.com/oant25/go_course/internal/http/middleware"
	"github.com/oant25/go_course/internal/http/ready"
	createhttp "github.com/oant25/go_course/internal/http/trip/create"
	finishhttp "github.com/oant25/go_course/internal/http/trip/finish"
	gethttp "github.com/oant25/go_course/internal/http/trip/get"
	"github.com/oant25/go_course/internal/infra/clock"
	"github.com/oant25/go_course/internal/infra/identifier"
	"github.com/oant25/go_course/internal/infra/postgres"
	"github.com/oant25/go_course/internal/infra/postgres/transaction"
	"github.com/oant25/go_course/internal/infra/telemetry"
	triprepo "github.com/oant25/go_course/internal/repository/trip"
	"github.com/oant25/go_course/internal/usecase/trip/create"
	"github.com/oant25/go_course/internal/usecase/trip/finish"
	"github.com/oant25/go_course/internal/usecase/trip/get"
)

func New(ctx context.Context, cfg config.Config) (*App, error) {
	logger := telemetry.NewLogger(os.Stdout, cfg.ServiceName, cfg.LogLevel)
	monitoring, err := telemetry.New(ctx, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("initialize telemetry: %w", err)
	}
	var pool *pgxpool.Pool
	abort := func(cause error) (*App, error) {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.ShutdownTimeout)
		defer cancel()
		cleanupErr := monitoring.Shutdown(cleanupCtx)
		if pool != nil {
			cleanupErr = errors.Join(cleanupErr, closePool(cleanupCtx, pool))
		}
		return nil, errors.Join(cause, cleanupErr)
	}
	poolCfg, err := postgres.PoolConfig(cfg.DB)
	if err != nil {
		return abort(fmt.Errorf("configure PostgreSQL: %w", err))
	}
	tracer, err := telemetry.NewDBTracer(monitoring.Traces, monitoring.Meters)
	if err != nil {
		return abort(fmt.Errorf("configure PostgreSQL tracing: %w", err))
	}
	poolCfg.ConnConfig.Tracer = tracer
	connectCtx, cancel := context.WithTimeout(ctx, cfg.DB.ConnectTimeout)
	defer cancel()
	pool, err = pgxpool.NewWithConfig(connectCtx, poolCfg)
	if err != nil {
		return abort(fmt.Errorf("create PostgreSQL pool: %w", err))
	}
	if err := pool.Ping(connectCtx); err != nil {
		return abort(fmt.Errorf("connect PostgreSQL: %w", err))
	}
	repo := triprepo.New(pool, cfg.DB.QueryTimeout)
	if err := monitoring.RegisterActive(repo, cfg); err != nil {
		return abort(fmt.Errorf("register active trips: %w", err))
	}
	tx := transaction.New(pool, cfg.DB.QueryTimeout)
	createTrip := create.New(repo, tx, clock.System{}, identifier.UUID{}, monitoring.Business)
	getTrip := get.New(repo)
	finishTrip := finish.New(repo, tx, clock.System{}, monitoring.Business)
	handlers := transport.NewServer(createhttp.New(createTrip), gethttp.New(getTrip), finishhttp.New(finishTrip), health.New(), ready.New(pool, cfg.DB.QueryTimeout))
	router, err := transport.NewRouter(handlers, logger, cfg.HTTP.ResponseBufferBytes, middleware.Providers{Traces: monitoring.Traces, Meters: monitoring.Meters})
	if err != nil {
		return abort(fmt.Errorf("create HTTP router: %w", err))
	}
	server := &http.Server{
		Addr: cfg.HTTP.Addr, Handler: router,
		ReadTimeout: cfg.HTTP.ReadTimeout, ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout, IdleTimeout: cfg.HTTP.IdleTimeout,
		ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}
	return &App{server: server, pool: pool, telemetry: monitoring, logger: logger, shutdownTimeout: cfg.ShutdownTimeout}, nil
}

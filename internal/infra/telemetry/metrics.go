package telemetry

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/metric"

	"github.com/oant25/go_course/internal/diagnostic"
)

type Reporter struct {
	logger   *slog.Logger
	interval time.Duration
	mu       sync.Mutex
	last     map[string]time.Time
}

func NewReporter(logger *slog.Logger, interval time.Duration) *Reporter {
	return &Reporter{logger: logger, interval: interval, last: make(map[string]time.Time)}
}

func (r *Reporter) Warn(ctx context.Context, component string, err error) {
	if err == nil {
		return
	}
	now := time.Now()
	r.mu.Lock()
	last := r.last[component]
	if !last.IsZero() && now.Sub(last) < r.interval {
		r.mu.Unlock()
		return
	}
	r.last[component] = now
	r.mu.Unlock()
	attrs := append([]slog.Attr{slog.String("component", component)}, diagnostic.Attributes(err)...)
	r.logger.LogAttrs(ctx, slog.LevelWarn, "telemetry degraded", attrs...)
}

type Business struct {
	meter              metric.Meter
	created, completed metric.Int64Counter
	logger             *slog.Logger
}

func NewBusiness(provider metric.MeterProvider, logger *slog.Logger) (*Business, error) {
	meter := provider.Meter("tripgo.business")
	created, err := meter.Int64Counter("tripgo.trips.created", metric.WithUnit("{trip}"))
	if err != nil {
		return nil, err
	}
	completed, err := meter.Int64Counter("tripgo.trips.completed", metric.WithUnit("{trip}"))
	if err != nil {
		return nil, err
	}
	return &Business{meter: meter, created: created, completed: completed, logger: logger}, nil
}

func (b *Business) TripCreated(ctx context.Context, id uuid.UUID) {
	b.created.Add(ctx, 1)
	b.logger.InfoContext(ctx, "trip created", "trip_id", id.String())
}

func (b *Business) TripCompleted(ctx context.Context, id uuid.UUID) {
	b.completed.Add(ctx, 1)
	b.logger.InfoContext(ctx, "trip completed", "trip_id", id.String())
}

func (b *Business) RegisterActive(reader activeTripReader, timeout time.Duration, reporter *Reporter) (metric.Registration, error) {
	gauge, err := b.meter.Int64ObservableGauge("tripgo.trips.active", metric.WithUnit("{trip}"))
	if err != nil {
		return nil, err
	}
	return b.meter.RegisterCallback(func(ctx context.Context, observer metric.Observer) error {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		value, err := reader.CountActive(ctx)
		if err != nil {
			reporter.Warn(ctx, "active_trips", err)
			return nil
		}
		observer.ObserveInt64(gauge, value)
		return nil
	}, gauge)
}

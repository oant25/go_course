//go:build integration

package trip_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/oant25/go_course/internal/infra/telemetry"
	triprepo "github.com/oant25/go_course/internal/repository/trip"
)

func TestActiveCountAndTelemetryRestartIntegration(t *testing.T) {
	pool := database(t)
	repo := triprepo.New(pool, testTimeout)
	service := newService(pool)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	if count, err := repo.CountActive(ctx); err != nil || count != 0 {
		t.Fatalf("empty count=%d err=%v", count, err)
	}
	first, err := service.Create(ctx, input())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(ctx, input()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Finish(ctx, first.Snapshot().ID); err != nil {
		t.Fatal(err)
	}
	if count, err := repo.CountActive(ctx); err != nil || count != 1 {
		t.Fatalf("active count=%d err=%v", count, err)
	}
	for range 2 {
		reader := sdkmetric.NewManualReader()
		provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
		logger := telemetry.NewLogger(io.Discard, "trip-service", slog.LevelInfo)
		business, err := telemetry.NewBusiness(provider, logger)
		if err != nil {
			t.Fatal(err)
		}
		registration, err := business.RegisterActive(repo, testTimeout, telemetry.NewReporter(logger, time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		var data metricdata.ResourceMetrics
		if err := reader.Collect(ctx, &data); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, scope := range data.ScopeMetrics {
			for _, m := range scope.Metrics {
				if m.Name == "tripgo.trips.active" {
					for _, point := range m.Data.(metricdata.Gauge[int64]).DataPoints {
						found = true
						if point.Value != 1 {
							t.Fatalf("gauge after restart=%d", point.Value)
						}
					}
				}
			}
		}
		if !found {
			t.Fatal("no active gauge observation")
		}
		if err := registration.Unregister(); err != nil {
			t.Fatal(err)
		}
		if err := provider.Shutdown(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

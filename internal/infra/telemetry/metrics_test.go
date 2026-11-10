package telemetry_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/mock/gomock"

	"github.com/oant25/go_course/internal/infra/telemetry"
)

func TestBusinessCountersAndActiveGauge(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() {
		if err := provider.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	logger := telemetry.NewLogger(io.Discard, "trip-service", slog.LevelInfo)
	business, err := telemetry.NewBusiness(provider, logger)
	if err != nil {
		t.Fatal(err)
	}
	business.TripCreated(ctx, uuid.New())
	business.TripCreated(ctx, uuid.New())
	business.TripCompleted(ctx, uuid.New())
	source := NewMockactiveTripReader(gomock.NewController(t))
	reporter := telemetry.NewReporter(logger, time.Minute)
	registration, err := business.RegisterActive(source, time.Second, reporter)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		value int64
		err   error
	}{{7, nil}, {0, nil}, {0, errors.New("database unavailable")}} {
		source.EXPECT().CountActive(gomock.Any()).DoAndReturn(func(ctx context.Context) (int64, error) {
			if _, ok := ctx.Deadline(); !ok {
				t.Fatal("gauge query lacks timeout")
			}
			return tc.value, tc.err
		})
		var data metricdata.ResourceMetrics
		if err := reader.Collect(ctx, &data); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, scope := range data.ScopeMetrics {
			for _, m := range scope.Metrics {
				switch m.Name {
				case "tripgo.trips.created", "tripgo.trips.completed":
					sum, ok := m.Data.(metricdata.Sum[int64])
					if !ok || len(sum.DataPoints) != 1 {
						t.Fatalf("counter: %+v", m)
					}
					want := int64(2)
					if m.Name == "tripgo.trips.completed" {
						want = 1
					}
					if sum.DataPoints[0].Value != want || sum.DataPoints[0].Attributes.Len() != 0 {
						t.Fatalf("counter=%+v", sum)
					}
				case "tripgo.trips.active":
					gauge, ok := m.Data.(metricdata.Gauge[int64])
					if !ok {
						t.Fatalf("gauge=%+v", m)
					}
					for _, point := range gauge.DataPoints {
						found = true
						if tc.err != nil || point.Value != tc.value || point.Attributes.Len() != 0 {
							t.Fatalf("stale/false gauge observation: %+v", point)
						}
					}
				}
			}
		}
		if found != (tc.err == nil) {
			t.Fatalf("gauge found=%v error=%v", found, tc.err)
		}
	}
	if err := registration.Unregister(); err != nil {
		t.Fatal(err)
	}
	var after metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &after); err != nil {
		t.Fatal(err)
	}
}

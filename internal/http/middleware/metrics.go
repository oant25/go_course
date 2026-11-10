package middleware

import (
	"net/http"
	"slices"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/oant25/go_course/internal/http/requeststate"
)

func Metrics(provider metric.MeterProvider) (func(http.Handler) http.Handler, error) {
	meter := provider.Meter("tripgo.http")
	duration, err := meter.Float64Histogram("http.server.request.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10))
	if err != nil {
		return nil, err
	}
	active, err := meter.Int64UpDownCounter("http.server.active_requests", metric.WithUnit("{request}"))
	if err != nil {
		return nil, err
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			state := requeststate.From(r.Context())
			attrs := []attribute.KeyValue{attribute.String("http.request.method", state.Method), attribute.String("http.route", state.Route)}
			active.Add(r.Context(), 1, metric.WithAttributes(attrs...))
			state.OnMetrics = func() {
				active.Add(state.Context, -1, metric.WithAttributes(attrs...))
				final := slices.Clone(attrs)
				final = append(final, attribute.Int("http.response.status_code", state.Status))
				duration.Record(state.Context, time.Since(state.Started).Seconds(), metric.WithAttributes(final...))
			}
			defer func() {
				if value := recover(); value != nil {
					state.Status = 500
					panic(value)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}, nil
}

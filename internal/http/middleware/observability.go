package middleware

import (
	"log/slog"
	"net/http"

	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"github.com/oant25/go_course/internal/http/domainerror"
)

type Providers struct {
	Traces trace.TracerProvider
	Meters metric.MeterProvider
}

func Observe(next http.Handler, logger *slog.Logger, limit int, resolve func(*http.Request) string, providers Providers) (http.Handler, error) {
	if providers.Traces == nil {
		providers.Traces = tracenoop.NewTracerProvider()
	}
	if providers.Meters == nil {
		providers.Meters = metricnoop.NewMeterProvider()
	}
	problems := domainerror.NewResponder()
	metrics, err := Metrics(providers.Meters)
	if err != nil {
		return nil, err
	}
	return Recover(problems, logger, limit, resolve)(Tracing(providers.Traces, problems, limit)(Logging(logger)(metrics(next)))), nil
}

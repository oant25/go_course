package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sort"
	"strings"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/oant25/go_course/internal/config"
)

type Runtime struct {
	Traces       *sdktrace.TracerProvider
	Meters       *sdkmetric.MeterProvider
	Business     *Business
	Reporter     *Reporter
	registration metric.Registration
	logger       *slog.Logger
	once         sync.Once
	shutdownErr  error
}

func New(ctx context.Context, cfg config.Config, logger *slog.Logger) (*Runtime, error) {
	settings := cfg.Telemetry
	reporter := NewReporter(logger, settings.ErrorLogInterval)
	initCtx, cancel := context.WithTimeout(ctx, settings.ExportTimeout)
	defer cancel()
	traces, err := traceExporter(initCtx, settings)
	if err != nil {
		return nil, fmt.Errorf("create trace exporter: %w", err)
	}
	metrics, err := metricExporter(initCtx, settings)
	if err != nil {
		cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), cfg.ShutdownTimeout)
		defer stop()
		return nil, errors.Join(fmt.Errorf("create metric exporter: %w", err), traces.Shutdown(cleanup))
	}
	res := Resource(cfg.ServiceName, settings.ResourceAttributes)
	tp := sdktrace.NewTracerProvider(sdktrace.WithResource(res), sdktrace.WithSampler(Sampler(settings)), sdktrace.WithBatcher(&safeTraceExporter{SpanExporter: traces, reporter: reporter},
		sdktrace.WithBatchTimeout(settings.BatchDelay), sdktrace.WithMaxQueueSize(settings.QueueSize), sdktrace.WithMaxExportBatchSize(settings.BatchSize), sdktrace.WithExportTimeout(settings.ExportTimeout)))
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithResource(res), sdkmetric.WithReader(sdkmetric.NewPeriodicReader(&safeMetricExporter{Exporter: metrics, reporter: reporter}, sdkmetric.WithInterval(settings.MetricInterval), sdkmetric.WithTimeout(settings.MetricTimeout))))
	runtime := &Runtime{Traces: tp, Meters: mp, Reporter: reporter, logger: logger}
	business, err := NewBusiness(mp, logger)
	if err != nil {
		cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), cfg.ShutdownTimeout)
		defer stop()
		return nil, errors.Join(err, runtime.Shutdown(cleanup))
	}
	runtime.Business = business
	logger.InfoContext(ctx, "telemetry started")
	return runtime, nil
}

func Resource(service string, values map[string]string) *resource.Resource {
	attrs := make([]attribute.KeyValue, 0, 1+len(values))
	attrs = append(attrs, attribute.String("service.name", service))
	keys := make([]string, 0, len(values))
	for key := range values {
		if key != "service.name" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		attrs = append(attrs, attribute.String(key, values[key]))
	}
	return resource.NewSchemaless(attrs...)
}

func Sampler(cfg config.Telemetry) sdktrace.Sampler {
	switch cfg.Sampler {
	case "always_on":
		return sdktrace.AlwaysSample()
	case "always_off":
		return sdktrace.NeverSample()
	case "traceidratio":
		return sdktrace.TraceIDRatioBased(cfg.SamplerRatio)
	case "parentbased_always_off":
		return sdktrace.ParentBased(sdktrace.NeverSample())
	case "parentbased_traceidratio":
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SamplerRatio))
	default:
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	}
}

func (r *Runtime) RegisterActive(reader activeTripReader, cfg config.Config) error {
	registration, err := r.Business.RegisterActive(reader, cfg.DB.QueryTimeout, r.Reporter)
	if err != nil {
		return err
	}
	r.registration = registration
	return nil
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	r.once.Do(func() {
		r.logger.InfoContext(ctx, "telemetry stopping")
		r.shutdownErr = r.Meters.Shutdown(ctx)
		if r.registration != nil {
			r.shutdownErr = errors.Join(r.shutdownErr, r.registration.Unregister())
		}
		r.shutdownErr = errors.Join(r.shutdownErr, r.Traces.Shutdown(ctx))
		if r.shutdownErr == nil {
			r.logger.InfoContext(ctx, "telemetry stopped")
		}
	})
	return r.shutdownErr
}

func traceExporter(ctx context.Context, cfg config.Telemetry) (sdktrace.SpanExporter, error) {
	if cfg.Protocol == "grpc" {
		return otlptracegrpc.New(ctx, otlptracegrpc.WithEndpointURL(cfg.Endpoint), otlptracegrpc.WithTimeout(cfg.ExportTimeout))
	}
	return otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(signalEndpoint(cfg.Endpoint, "traces")), otlptracehttp.WithTimeout(cfg.ExportTimeout))
}

func metricExporter(ctx context.Context, cfg config.Telemetry) (sdkmetric.Exporter, error) {
	if cfg.Protocol == "grpc" {
		return otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithEndpointURL(cfg.Endpoint), otlpmetricgrpc.WithTimeout(cfg.ExportTimeout))
	}
	return otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpointURL(signalEndpoint(cfg.Endpoint, "metrics")), otlpmetrichttp.WithTimeout(cfg.ExportTimeout))
}

func signalEndpoint(endpoint, signal string) string {
	value, _ := url.Parse(endpoint)
	value.Path = strings.TrimRight(value.Path, "/") + "/v1/" + signal
	return value.String()
}

type safeTraceExporter struct {
	sdktrace.SpanExporter
	reporter *Reporter
}

func (e *safeTraceExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	if err := e.SpanExporter.ExportSpans(ctx, spans); err != nil {
		e.reporter.Warn(ctx, "trace_export", err)
	}
	return nil
}

type safeMetricExporter struct {
	sdkmetric.Exporter
	reporter *Reporter
}

func (e *safeMetricExporter) Export(ctx context.Context, data *metricdata.ResourceMetrics) error {
	if err := e.Exporter.Export(ctx, data); err != nil {
		e.reporter.Warn(ctx, "metric_export", err)
	}
	return nil
}

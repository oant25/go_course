package config

import (
	"errors"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Telemetry struct {
	ResourceAttributes map[string]string
	Endpoint           string
	Protocol           string
	Sampler            string
	SamplerRatio       float64
	ExportTimeout      time.Duration
	MetricInterval     time.Duration
	MetricTimeout      time.Duration
	BatchDelay         time.Duration
	QueueSize          int
	BatchSize          int
	ErrorLogInterval   time.Duration
}

func (r *reader) telemetry() Telemetry {
	cfg := Telemetry{
		Endpoint: r.string("OTEL_EXPORTER_OTLP_ENDPOINT"), Protocol: r.string("OTEL_EXPORTER_OTLP_PROTOCOL"),
		Sampler: r.string("OTEL_TRACES_SAMPLER"), ExportTimeout: r.milliseconds("OTEL_EXPORTER_OTLP_TIMEOUT"),
		MetricInterval: r.milliseconds("OTEL_METRIC_EXPORT_INTERVAL"), MetricTimeout: r.milliseconds("OTEL_METRIC_EXPORT_TIMEOUT"),
		BatchDelay: r.milliseconds("OTEL_BSP_SCHEDULE_DELAY"), QueueSize: int(r.int32("OTEL_BSP_MAX_QUEUE_SIZE", 1)),
		BatchSize: int(r.int32("OTEL_BSP_MAX_EXPORT_BATCH_SIZE", 1)), ErrorLogInterval: r.duration("TELEMETRY_ERROR_LOG_INTERVAL"),
		ResourceAttributes: make(map[string]string),
	}
	endpoint, err := url.Parse(cfg.Endpoint)
	if err != nil || endpoint.Hostname() == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		r.fail("OTEL_EXPORTER_OTLP_ENDPOINT", errors.New("must be an http(s) URL without credentials, query or fragment"))
	}
	if cfg.Protocol != "grpc" && cfg.Protocol != "http/protobuf" {
		r.fail("OTEL_EXPORTER_OTLP_PROTOCOL", errors.New("must be grpc or http/protobuf"))
	}
	if err == nil && cfg.Protocol == "grpc" && endpoint.Path != "" && endpoint.Path != "/" {
		r.fail("OTEL_EXPORTER_OTLP_ENDPOINT", errors.New("gRPC endpoint must not include a path"))
	}
	if cfg.MetricTimeout > cfg.MetricInterval {
		r.fail("OTEL_METRIC_EXPORT_TIMEOUT", errors.New("must not exceed OTEL_METRIC_EXPORT_INTERVAL"))
	}
	if cfg.BatchSize > cfg.QueueSize {
		r.fail("OTEL_BSP_MAX_EXPORT_BATCH_SIZE", errors.New("must not exceed OTEL_BSP_MAX_QUEUE_SIZE"))
	}
	switch cfg.Sampler {
	case "always_on", "always_off", "parentbased_always_on", "parentbased_always_off":
	case "traceidratio", "parentbased_traceidratio":
		ratio, err := strconv.ParseFloat(r.string("OTEL_TRACES_SAMPLER_ARG"), 64)
		if err != nil || math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio < 0 || ratio > 1 {
			r.fail("OTEL_TRACES_SAMPLER_ARG", errors.New("must be a finite number from 0 to 1"))
		} else {
			cfg.SamplerRatio = ratio
		}
	default:
		r.fail("OTEL_TRACES_SAMPLER", errors.New("unsupported sampler"))
	}
	cfg.ResourceAttributes = r.resourceAttributes()

	return cfg
}

func (r *reader) milliseconds(name string) time.Duration {
	raw := r.string(name)
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 || value > int64(math.MaxInt64)/int64(time.Millisecond) {
		r.fail(name, errors.New("must be a positive integer number of milliseconds within duration range"))
		return 0
	}
	return time.Duration(value) * time.Millisecond
}

func (r *reader) resourceAttributes() map[string]string {
	attributes := make(map[string]string)
	for _, pair := range strings.Split(r.string("OTEL_RESOURCE_ATTRIBUTES"), ",") {
		key, value, ok := strings.Cut(pair, "=")
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if !ok || key == "" || value == "" {
			r.fail("OTEL_RESOURCE_ATTRIBUTES", errors.New("expected nonempty key=value entries"))
			continue
		}
		if _, exists := attributes[key]; exists {
			r.fail("OTEL_RESOURCE_ATTRIBUTES", errors.New("duplicate key"))
			continue
		}
		if key == "service.name" {
			r.fail("OTEL_RESOURCE_ATTRIBUTES", errors.New("set service.name through OTEL_SERVICE_NAME"))
			continue
		}
		attributes[key] = value
	}
	for _, key := range []string{"service.namespace", "deployment.environment", "service.version"} {
		if attributes[key] == "" {
			r.fail("OTEL_RESOURCE_ATTRIBUTES", errors.New("missing "+key))
		}
	}
	return attributes
}

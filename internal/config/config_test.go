package config_test

import (
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/oant25/go_course/internal/config"
)

func validEnvironment(t *testing.T) map[string]string {
	t.Helper()
	//nolint:gosec // Фиктивный пароль проверяет, что ошибки конфигурации не раскрывают DSN.
	values := map[string]string{
		resourceAttributesEnv: "service.namespace=tripgo,deployment.environment=local,service.version=test",
		exportEndpointEnv:     "http://collector.invalid:4318", "OTEL_EXPORTER_OTLP_PROTOCOL": "http/protobuf",
		"OTEL_TRACES_SAMPLER": "parentbased_always_on", exportTimeoutEnv: "5000",
		"OTEL_METRIC_EXPORT_INTERVAL": "10000", "OTEL_METRIC_EXPORT_TIMEOUT": "5000",
		"OTEL_BSP_SCHEDULE_DELAY": "5000", "OTEL_BSP_MAX_QUEUE_SIZE": "2048", "OTEL_BSP_MAX_EXPORT_BATCH_SIZE": "512",
		"TELEMETRY_ERROR_LOG_INTERVAL": "30s",

		"OTEL_SERVICE_NAME": "trip-service", "LOG_LEVEL": "info", "SHUTDOWN_TIMEOUT": "10s",
		"HTTP_ADDR": ":8080", "HTTP_RESPONSE_BUFFER_BYTES": "1048576",
		"HTTP_READ_TIMEOUT": "5s", "HTTP_READ_HEADER_TIMEOUT": "2s", "HTTP_WRITE_TIMEOUT": "10s", "HTTP_IDLE_TIMEOUT": "60s",
		"DATABASE_URL": "postgres://user:secret_marker@localhost:5432/trips", maxConnsEnv: "10", minConnsEnv: "0",
		"DATABASE_MAX_CONN_LIFETIME": "30m", "DATABASE_CONNECT_TIMEOUT": "5s", "DATABASE_QUERY_TIMEOUT": "3s",
	}
	for key, value := range values {
		t.Setenv(key, value)
	}
	return values
}

func TestLoad(t *testing.T) {
	validEnvironment(t)
	t.Setenv("OTEL_SERVICE_NAME", "  trip-service  ")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServiceName != "trip-service" || cfg.LogLevel != slog.LevelInfo || cfg.HTTP.ResponseBufferBytes != 1<<20 || cfg.HTTP.Addr != ":8080" || cfg.DB.MinConns != 0 || cfg.DB.MaxConns != 10 || cfg.DB.QueryTimeout != 3*time.Second || cfg.ShutdownTimeout != 10*time.Second {
		t.Fatalf("unexpected parsed configuration (database URL omitted)")
	}
}

func TestLoadRequiresEveryVariable(t *testing.T) {
	values := validEnvironment(t)
	for name := range values {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, "")
			_, err := config.Load()
			var invalid *config.Error
			if !errors.As(err, &invalid) || !strings.Contains(err.Error(), name) {
				t.Fatalf("missing %s: %v", name, err)
			}
			if strings.Contains(err.Error(), "secret_marker") {
				t.Fatal("configuration error leaked database URL")
			}
		})
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	validEnvironment(t)
	for _, tc := range []struct{ name, value string }{
		{"LOG_LEVEL", "verbose"},
		{"OTEL_SERVICE_NAME", " \t"},
		{"SHUTDOWN_TIMEOUT", "0s"},
		{"HTTP_READ_TIMEOUT", "-1s"},
		{"HTTP_IDLE_TIMEOUT", "broken"},
		{maxConnsEnv, "0"},
		{minConnsEnv, "-1"},
		{minConnsEnv, "11"},
		{maxConnsEnv, "2147483648"},
		{maxConnsEnv, "1.5"},
		{"HTTP_RESPONSE_BUFFER_BYTES", "1023"},
		{"DATABASE_QUERY_TIMEOUT", "0"},
	} {
		t.Run(tc.name+"="+tc.value, func(t *testing.T) {
			t.Setenv(tc.name, tc.value)
			_, err := config.Load()
			var invalid *config.Error
			if !errors.As(err, &invalid) || !strings.Contains(err.Error(), tc.name) {
				t.Fatalf("invalid value accepted or unclear error: %v", err)
			}
		})
	}
}

func TestLoadAggregatesErrors(t *testing.T) {
	validEnvironment(t)
	t.Setenv("HTTP_ADDR", "")
	t.Setenv(maxConnsEnv, "0")
	_, err := config.Load()
	if err == nil || !strings.Contains(err.Error(), "HTTP_ADDR") || !strings.Contains(err.Error(), maxConnsEnv) {
		t.Fatalf("errors not aggregated: %v", err)
	}
}

const (
	resourceAttributesEnv = "OTEL_RESOURCE_ATTRIBUTES"
	exportEndpointEnv     = "OTEL_EXPORTER_OTLP_ENDPOINT"
	exportTimeoutEnv      = "OTEL_EXPORTER_OTLP_TIMEOUT"
	maxConnsEnv           = "DATABASE_MAX_CONNS"
)

const (
	minConnsEnv = "DATABASE_MIN_CONNS"
)

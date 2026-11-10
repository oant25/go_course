package telemetry_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/trace"

	"github.com/oant25/go_course/internal/infra/telemetry"
)

func TestLoggerCorrelationAndGroups(t *testing.T) {
	t.Parallel()
	for _, withSpan := range []bool{false, true} {
		t.Run(map[bool]string{false: "startup", true: "request"}[withSpan], func(t *testing.T) {
			var output bytes.Buffer
			logger := telemetry.NewLogger(&output, "trip-service", slog.LevelInfo)
			ctx := context.Background()
			traceID, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
			spanID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
			if withSpan {
				ctx = trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID}))
			}
			logger.With("service", "spoofed", "component", "http").WithGroup("details").With("trace_id", "spoofed").InfoContext(ctx, "request handled", "status", 200, "span_id", "spoofed")
			var got map[string]any
			if err := json.Unmarshal(output.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"time", "level", "msg", "service", "trace_id", "span_id"} {
				if _, ok := got[key]; !ok {
					t.Fatalf("missing %s: %s", key, output.String())
				}
				if strings.Count(output.String(), `"`+key+`":`) != 1 {
					t.Fatalf("duplicate key %s", key)
				}
			}
			if got["service"] != "trip-service" || strings.Contains(output.String(), "spoofed") {
				t.Fatalf("reserved fields overwritten: %s", output.String())
			}
			if withSpan {
				if got["trace_id"] != traceID.String() || got["span_id"] != spanID.String() {
					t.Fatal("lost correlation")
				}
			} else if got["trace_id"] != "" || got["span_id"] != "" {
				t.Fatal("fabricated correlation")
			}
		})
	}
}

func TestReporterRateLimitsAndSanitizes(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	reporter := telemetry.NewReporter(telemetry.NewLogger(&output, "trip-service", slog.LevelInfo), time.Hour)
	err := &pgconn.PgError{Code: "23505", Message: "password=secret_marker", Detail: "driver secret_marker"}
	reporter.Warn(context.Background(), "metric_export", err)
	reporter.Warn(context.Background(), "metric_export", errors.New("secret_marker"))
	if strings.Count(output.String(), "telemetry degraded") != 1 || strings.Contains(output.String(), "secret_marker") || !strings.Contains(output.String(), "23505") {
		t.Fatalf("unsafe or duplicate log: %s", output.String())
	}
}

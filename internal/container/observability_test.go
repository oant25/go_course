package container_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/oant25/go_course/internal/domain/trip"
	"github.com/oant25/go_course/internal/http/domainerror"
	"github.com/oant25/go_course/internal/http/middleware"
	"github.com/oant25/go_course/internal/infra/telemetry"
)

func TestObservedHTTPFinalization(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		status int
	}{
		{"success", 201}, {"validation", 400}, {"conflict", 409}, {"failure", 500}, {"panic", 500}, {"partial panic", 500}, {"overflow", 500}, {abortScenario, 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			var logs bytes.Buffer
			logger := telemetry.NewLogger(&logs, "trip-service", slog.LevelInfo)
			reader := sdkmetric.NewManualReader()
			mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
			exporter := tracetest.NewInMemoryExporter()
			tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
			t.Cleanup(func() {
				if err := mp.Shutdown(ctx); err != nil {
					t.Error(err)
				}
				if err := tp.Shutdown(ctx); err != nil {
					t.Error(err)
				}
			})
			responder := domainerror.NewResponder()
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assertActiveRequest(t, ctx, reader)
				serveScenario(t, responder, tc.name, w, r)
			})
			handler, err := middleware.Observe(next, logger, 1024, func(*http.Request) string { return "/api/v1/trips/{tripId}" }, middleware.Providers{Traces: tp, Meters: mp})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest("GET", "/api/v1/trips/10000000-0000-0000-0000-000000000001", nil)
			request.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
			response := httptest.NewRecorder()
			var panicked any
			func() { defer func() { panicked = recover() }(); handler.ServeHTTP(response, request) }()
			if tc.name == abortScenario {
				if panicked != http.ErrAbortHandler || response.Body.Len() != 0 {
					t.Fatalf("abort=%v body=%s", panicked, response.Body.String())
				}
			} else if panicked != nil || response.Code != tc.status {
				t.Fatalf("status=%d panic=%v body=%s", response.Code, panicked, response.Body.String())
			}
			if tc.status == 500 && response.Header().Get("Location") != "" {
				t.Fatal("partial response headers escaped")
			}
			if strings.Contains(logs.String()+response.Body.String(), "secret_marker") {
				t.Fatal("private error leaked")
			}
			assertHTTPTraceAndLog(t, &logs, exporter, tc.status, tc.name)
			assertHTTPMeasurements(t, ctx, reader, tc.status)
		})
	}
}

type failingWriter struct{ header http.Header }

func (w *failingWriter) Header() http.Header       { return w.header }
func (w *failingWriter) WriteHeader(int)           {}
func (w *failingWriter) Write([]byte) (int, error) { return 0, errors.New("secret_marker") }
func TestNetworkWriteErrorLoggedOnce(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	logger := telemetry.NewLogger(&logs, "trip-service", slog.LevelInfo)
	handler, err := middleware.Observe(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte("ok")); err != nil {
			t.Error(err)
		}
	}), logger, 1024, func(*http.Request) string { return "/health" }, middleware.Providers{})
	if err != nil {
		t.Fatal(err)
	}
	handler.ServeHTTP(&failingWriter{header: make(http.Header)}, httptest.NewRequest("GET", "/health", nil))
	if strings.Count(logs.String(), "request handled") != 1 || !strings.Contains(logs.String(), "write_error") || strings.Contains(logs.String(), "secret_marker") {
		t.Fatalf("write error log: %s", logs.String())
	}
}

func assertActiveRequest(t *testing.T, ctx context.Context, reader *sdkmetric.ManualReader) {
	t.Helper()
	var during metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &during); err != nil {
		t.Fatal(err)
	}
	active := int64(0)
	for _, scope := range during.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name == "http.server.active_requests" {
				for _, point := range m.Data.(metricdata.Sum[int64]).DataPoints {
					active += point.Value
				}
			}
		}
	}
	if active != 1 {
		t.Fatalf("active during request=%d", active)
	}
}

func serveScenario(t *testing.T, responder *domainerror.Responder, scenario string, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	switch scenario {
	case "validation":
		responder.WriteInvalidRequest(w, r, errors.New("secret_marker"))
	case "conflict":
		responder.Write(w, r, trip.ErrDriverBusy)
	case "failure":
		responder.Write(w, r, errors.New("secret_marker"))
	case "panic":
		panic("secret_marker")
	case "partial panic":
		w.Header().Set("Location", "/partial")
		w.WriteHeader(201)
		if _, err := w.Write([]byte(`{"partial":`)); err != nil {
			t.Fatal(err)
		}
		panic("secret_marker")
	case "overflow":
		w.WriteHeader(201)
		_, err := w.Write([]byte(strings.Repeat("x", 2048)))
		if err == nil {
			t.Fatal("overflow accepted")
		}
	case abortScenario:
		panic(http.ErrAbortHandler)
	default:
		w.WriteHeader(201)
		if _, err := w.Write([]byte(`{"ok":true}`)); err != nil {
			t.Fatal(err)
		}
	}
}

func assertHTTPTraceAndLog(t *testing.T, logs *bytes.Buffer, exporter *tracetest.InMemoryExporter, status int, scenario string) {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(logs.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("access records=%d: %s", len(lines), logs.String())
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatal(err)
	}
	if record["msg"] != "request handled" || record["trace_id"] != "4bf92f3577b34da6a3ce929d0e0e4736" || record["span_id"] == "" || record["status"] != float64(status) {
		t.Fatalf("access=%v", record)
	}
	level := "INFO"
	if status >= 500 {
		level = "ERROR"
	}
	if record["level"] != level {
		t.Fatalf("level=%v", record["level"])
	}
	if strings.Contains(scenario, "panic") && record["stack"] == nil {
		t.Fatal("missing panic stack")
	}
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("HTTP spans=%d", len(spans))
	}
	span := spans[0]
	if span.Name != "GET /api/v1/trips/{tripId}" || span.Parent.SpanID().String() != "00f067aa0ba902b7" || span.SpanContext.SpanID().String() != record["span_id"] {
		t.Fatalf("span=%+v", span)
	}
	if status >= 400 && span.Status.Code != codes.Error {
		t.Fatalf("error span status=%v", span.Status)
	}
	statusFound := false
	for _, attr := range span.Attributes {
		if attr.Key == attribute.Key("http.response.status_code") && attr.Value.AsInt64() == int64(status) {
			statusFound = true
		}
	}
	if !statusFound {
		t.Fatalf("final HTTP status missing in span: %v", span.Attributes)
	}
}

func assertHTTPMeasurements(t *testing.T, ctx context.Context, reader *sdkmetric.ManualReader, wantStatus int) {
	t.Helper()
	var data metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &data); err != nil {
		t.Fatal(err)
	}
	durationCount := uint64(0)
	active := int64(0)
	activeFound := false
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			switch m.Name {
			case "http.server.request.duration":
				for _, point := range m.Data.(metricdata.Histogram[float64]).DataPoints {
					durationCount += point.Count
					route, _ := point.Attributes.Value("http.route")
					status, _ := point.Attributes.Value("http.response.status_code")
					if route.AsString() != "/api/v1/trips/{tripId}" || status.AsInt64() != int64(wantStatus) {
						t.Fatalf("metric labels=%v", point.Attributes)
					}
				}
			case "http.server.active_requests":
				activeFound = true
				for _, point := range m.Data.(metricdata.Sum[int64]).DataPoints {
					active += point.Value
				}
			}
		}
	}
	if durationCount != 1 || !activeFound || active != 0 {
		t.Fatalf("duration=%d active=%d found=%v", durationCount, active, activeFound)
	}
}

const (
	abortScenario = "abort"
)

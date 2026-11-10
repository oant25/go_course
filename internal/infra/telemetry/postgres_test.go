package telemetry_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/oant25/go_course/internal/infra/telemetry"
)

func TestSQLMetadata(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ sql, op, table string }{
		{"SELECT COUNT(*) FROM trips WHERE status=$1", selectOperation, tripsTable},
		{`SELECT id FROM "public"."trips" WHERE id=$1 FOR UPDATE`, selectOperation, tripsTable},
		{"INSERT INTO trip_status_history (trip_id) VALUES ($1)", "INSERT", "trip_status_history"},
		{`UPDATE "trips" SET status=$1 WHERE id=$2 RETURNING id`, "UPDATE", tripsTable},
		{"begin", "BEGIN", ""},
		{"commit", "COMMIT", ""},
		{"rollback", "ROLLBACK", ""},
		{"SELECT * FROM secret_marker", selectOperation, ""},
		{"VACUUM secret_marker", "UNKNOWN", ""},
	} {
		op, table := telemetry.SQLMetadata(tc.sql)
		if op != tc.op || table != tc.table {
			t.Fatalf("%s: %s %s", tc.sql, op, table)
		}
	}
	for _, sql := range []string{"SELECT 'secret_marker' FROM trips", "SELECT 123 FROM trips", "SELECT * FROM trips -- secret_marker", "SELECT $$secret_marker$$ FROM trips"} {
		if telemetry.SafeSQL(sql) != "" {
			t.Fatalf("unsafe SQL exposed: %s", sql)
		}
	}
	if telemetry.SafeSQL("SELECT id FROM trips WHERE id=$1") == "" {
		t.Fatal("safe parameterized SQL omitted")
	}
}

func TestDatabaseSpanIsChildAndErrorIsSanitized(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() {
		if err := mp.Shutdown(ctx); err != nil {
			t.Error(err)
		}
		if err := tp.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	tracer, err := telemetry.NewDBTracer(tp, mp)
	if err != nil {
		t.Fatal(err)
	}
	parentCtx, parent := tp.Tracer("test").Start(ctx, "HTTP")
	sql := "SELECT id FROM trips WHERE id=$1"
	queryCtx := tracer.TraceQueryStart(parentCtx, nil, pgx.TraceQueryStartData{SQL: sql, Args: []any{privateMarker}})
	failure := &pgconn.PgError{Code: "23505", Message: privateMarker, Detail: privateMarker}
	tracer.TraceQueryEnd(queryCtx, nil, pgx.TraceQueryEndData{Err: failure})
	parent.End()
	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("spans=%d", len(spans))
	}
	child := spans[0]
	if child.Parent.SpanID() != parent.SpanContext().SpanID() || child.Status.Code != codes.Error {
		t.Fatalf("child=%+v", child)
	}
	attrs := map[string]string{}
	for _, a := range child.Attributes {
		attrs[string(a.Key)] = a.Value.AsString()
	}
	if attrs["db.operation.name"] != selectOperation || attrs["db.collection.name"] != tripsTable || attrs["error.type"] == "" {
		t.Fatalf("attributes=%v", attrs)
	}
	encoded, err := json.Marshal(spans)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), privateMarker) {
		t.Fatal("SQL arguments or database error leaked")
	}
	if failure.Detail != privateMarker {
		t.Fatal("driver error was mutated")
	}
	var data metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &data); err != nil {
		t.Fatal(err)
	}
	count := uint64(0)
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name == "db.client.operation.duration" {
				hist := m.Data.(metricdata.Histogram[float64])
				for _, point := range hist.DataPoints {
					count += point.Count
				}
			}
		}
	}
	if count != 1 {
		t.Fatalf("DB duration count=%d", count)
	}
}

func TestDatabaseQueryWithoutHTTPParentHasOneRootSpan(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() {
		if err := mp.Shutdown(ctx); err != nil {
			t.Error(err)
		}
		if err := tp.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	tracer, err := telemetry.NewDBTracer(tp, mp)
	if err != nil {
		t.Fatal(err)
	}
	queryCtx := tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{SQL: "SELECT COUNT(*) FROM trips WHERE status=$1", Args: []any{"active"}})
	tracer.TraceQueryEnd(queryCtx, nil, pgx.TraceQueryEndData{})
	spans := exporter.GetSpans()
	if len(spans) != 1 || spans[0].Parent.IsValid() || spans[0].Name != selectOperation {
		t.Fatalf("root spans=%+v", spans)
	}
}

const (
	selectOperation = "SELECT"
	privateMarker   = "secret_marker"
)

const (
	tripsTable = "trips"
)

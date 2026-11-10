package telemetry

import (
	"context"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"

	"github.com/oant25/go_course/internal/diagnostic"
)

type DBTracer struct {
	*otelpgx.Tracer
	duration metric.Float64Histogram
	root     trace.Tracer
}
type (
	queryKey   struct{}
	queryState struct {
		start                 time.Time
		operation, collection string
		delegated             bool
	}
)

func NewDBTracer(tp trace.TracerProvider, mp metric.MeterProvider) (*DBTracer, error) {
	duration, err := mp.Meter("tripgo.postgres").Float64Histogram("db.client.operation.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(.001, .005, .01, .05, .1, .5, 1, 5, 10))
	if err != nil {
		return nil, err
	}
	tracer := otelpgx.NewTracer(otelpgx.WithTracerProvider(tp), otelpgx.WithMeterProvider(metricnoop.NewMeterProvider()), otelpgx.WithDisableSQLStatementInAttributes(), otelpgx.WithDisableConnectionDetailsInAttributes(), otelpgx.WithDisableAcquireTracer(), otelpgx.WithSpanNameFunc(func(sql string) string { op, _ := SQLMetadata(sql); return op }))
	return &DBTracer{Tracer: tracer, duration: duration, root: tp.Tracer("tripgo.postgres")}, nil
}

func (t *DBTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	state := queryState{start: time.Now()}
	state.operation, state.collection = SQLMetadata(data.SQL)
	state.delegated = trace.SpanFromContext(ctx).IsRecording()
	if state.delegated {
		ctx = t.Tracer.TraceQueryStart(ctx, conn, data)
	} else {
		ctx, _ = t.root.Start(ctx, state.operation, trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(attribute.String("db.system.name", "postgresql")))
	}
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("db.operation.name", state.operation))
	if state.collection != "" {
		span.SetAttributes(attribute.String("db.collection.name", state.collection))
	}
	if sql := SafeSQL(data.SQL); sql != "" {
		span.SetAttributes(attribute.String("db.query.text", sql))
	}
	return context.WithValue(ctx, queryKey{}, state)
}

func (t *DBTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	if state, ok := ctx.Value(queryKey{}).(queryState); ok {
		attrs := []attribute.KeyValue{attribute.String("db.operation.name", state.operation)}
		if state.collection != "" {
			attrs = append(attrs, attribute.String("db.collection.name", state.collection))
		}
		if data.Err != nil {
			attrs = append(attrs, attribute.String("error.type", diagnostic.Kind(data.Err)))
		}
		t.duration.Record(ctx, time.Since(state.start).Seconds(), metric.WithAttributes(attrs...))
	}
	markDBError(ctx, data.Err)
	data.Err = diagnostic.Safe(data.Err)
	state, _ := ctx.Value(queryKey{}).(queryState)
	if state.delegated {
		t.Tracer.TraceQueryEnd(ctx, conn, data)
	} else {
		span := trace.SpanFromContext(ctx)
		if data.Err != nil {
			span.RecordError(data.Err)
		}
		span.End()
	}
}

func markDBError(ctx context.Context, err error) {
	if err == nil {
		return
	}
	span := trace.SpanFromContext(ctx)
	span.SetStatus(codes.Error, diagnostic.Kind(err))
	span.SetAttributes(attribute.String("error.type", diagnostic.Kind(err)))
	if state := diagnostic.SQLState(err); state != "" {
		span.SetAttributes(attribute.String("db.response.status_code", state))
	}
}

func (t *DBTracer) TraceConnectEnd(ctx context.Context, data pgx.TraceConnectEndData) {
	markDBError(ctx, data.Err)
	data.Err = diagnostic.Safe(data.Err)
	t.Tracer.TraceConnectEnd(ctx, data)
}

func (t *DBTracer) TracePrepareStart(ctx context.Context, conn *pgx.Conn, data pgx.TracePrepareStartData) context.Context {
	data.Name = ""
	return t.Tracer.TracePrepareStart(ctx, conn, data)
}

func (t *DBTracer) TracePrepareEnd(ctx context.Context, conn *pgx.Conn, data pgx.TracePrepareEndData) {
	markDBError(ctx, data.Err)
	data.Err = diagnostic.Safe(data.Err)
	t.Tracer.TracePrepareEnd(ctx, conn, data)
}

func (t *DBTracer) TraceBatchQuery(ctx context.Context, conn *pgx.Conn, data pgx.TraceBatchQueryData) {
	data.Err = diagnostic.Safe(data.Err)
	t.Tracer.TraceBatchQuery(ctx, conn, data)
}

func (t *DBTracer) TraceBatchEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceBatchEndData) {
	markDBError(ctx, data.Err)
	data.Err = diagnostic.Safe(data.Err)
	t.Tracer.TraceBatchEnd(ctx, conn, data)
}

func (t *DBTracer) TraceCopyFromStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceCopyFromStartData) context.Context {
	if len(data.TableName) != 1 || (data.TableName[0] != "trips" && data.TableName[0] != "trip_status_history") {
		data.TableName = pgx.Identifier{"unknown"}
	}
	return t.Tracer.TraceCopyFromStart(ctx, conn, data)
}

func (t *DBTracer) TraceCopyFromEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceCopyFromEndData) {
	markDBError(ctx, data.Err)
	data.Err = diagnostic.Safe(data.Err)
	t.Tracer.TraceCopyFromEnd(ctx, conn, data)
}

func (t *DBTracer) TraceAcquireEnd(ctx context.Context, pool *pgxpool.Pool, data pgxpool.TraceAcquireEndData) {
	data.Err = diagnostic.Safe(data.Err)
	t.Tracer.TraceAcquireEnd(ctx, pool, data)
}

package middleware

import (
	"errors"
	"net/http"
	"runtime/debug"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/oant25/go_course/internal/http/domainerror"
	"github.com/oant25/go_course/internal/http/requeststate"
	"github.com/oant25/go_course/internal/http/response"
)

func Tracing(provider trace.TracerProvider, problems *domainerror.Responder, limit int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		adapter := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			state := requeststate.From(r.Context())
			state.Context = r.Context()
			local := response.New(limit)
			defer func() {
				if value := recover(); value != nil {
					state.Panic = value
					if value == http.ErrAbortHandler {
						state.Aborted = true
						state.Status = 500
						state.Code = "request_aborted"
						state.Err = http.ErrAbortHandler
						local.Reset()
						local.WriteHeader(500)
					} else {
						problems.WritePanic(local, r, debug.Stack())
					}
				}
				if err := local.Err(); err != nil {
					problems.Write(local, r, err)
				}
				state.Status = local.Status()
				if state.Status >= 400 && state.Code == "" {
					state.Code = "internal_error"
					state.Err = domainerror.ErrInternal
				}
				state.FinishMetrics()
				span := trace.SpanFromContext(r.Context())
				span.SetAttributes(attribute.String("http.route", state.Route))
				if state.Code != "" {
					span.SetStatus(codes.Error, state.Code)
					span.SetAttributes(attribute.String("error.type", state.Code))
					span.RecordError(errors.New(state.Code))
				}
				if err := local.Commit(w); err != nil {
					state.WriteError = err
				}
				state.Prepared = true
			}()
			next.ServeHTTP(local, r)
		})
		observed := otelhttp.NewHandler(adapter, "trip-service", otelhttp.WithTracerProvider(provider), otelhttp.WithMeterProvider(metricnoop.NewMeterProvider()), otelhttp.WithPropagators(propagation.TraceContext{}), otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			state := requeststate.From(r.Context())
			return state.Method + " " + state.Route
		}))
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			observed.ServeHTTP(w, r)
			if state := requeststate.From(r.Context()); state.Panic != nil {
				panic(state.Panic)
			}
		})
	}
}

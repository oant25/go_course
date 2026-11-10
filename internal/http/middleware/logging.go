package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/oant25/go_course/internal/diagnostic"
	"github.com/oant25/go_course/internal/http/requeststate"
)

func logRequest(state *requeststate.State, logger *slog.Logger) {
	level := slog.LevelInfo
	if state.Status >= 500 || state.WriteError != nil {
		level = slog.LevelError
	}
	attrs := []slog.Attr{slog.String("method", state.Method), slog.String("route", state.Route), slog.Int("status", state.Status), slog.Float64("duration_ms", float64(time.Since(state.Started))/float64(time.Millisecond))}
	if state.Code != "" {
		attrs = append(attrs, slog.String("code", state.Code))
	}
	attrs = append(attrs, diagnostic.Attributes(state.Err)...)
	if len(state.Stack) > 0 {
		attrs = append(attrs, slog.String("stack", string(state.Stack)))
	}
	if state.WriteError != nil {
		attrs = append(attrs, slog.String("write_error", diagnostic.Kind(state.WriteError)))
	}
	logger.LogAttrs(state.Context, level, "request handled", attrs...)
}

func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			state := requeststate.From(r.Context())
			state.OnLog = func() { logRequest(state, logger) }
			next.ServeHTTP(w, r)
		})
	}
}

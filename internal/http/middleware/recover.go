package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/oant25/go_course/internal/http/domainerror"
	"github.com/oant25/go_course/internal/http/requeststate"
	"github.com/oant25/go_course/internal/http/response"
)

func Recover(problems *domainerror.Responder, logger *slog.Logger, bufferLimit int, resolve ...func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			state := &requeststate.State{Context: r.Context(), Method: boundedMethod(r.Method), Route: "unmatched", Started: time.Now(), Status: 200}
			r = r.WithContext(requeststate.With(r.Context(), state))
			state.Context = r.Context()
			state.OnLog = func() { logRequest(state, logger) }
			buffer := response.New(bufferLimit)
			defer func() {
				if value := recover(); value != nil {
					if value == http.ErrAbortHandler {
						state.Aborted = true
						state.Status = 500
						state.Code = "request_aborted"
						state.Err = http.ErrAbortHandler
					} else if !state.Prepared {
						problems.WritePanic(buffer, r, debug.Stack())
						state.Status = buffer.Status()
					}
				}
				if state.Aborted {
					state.FinishMetrics()
					state.FinishLog()
					panic(http.ErrAbortHandler)
				}
				if err := buffer.Err(); err != nil {
					problems.Write(buffer, r, err)
				}
				state.Status = buffer.Status()
				state.FinishMetrics()
				if err := buffer.Commit(w); err != nil {
					state.WriteError = err
				}
				state.FinishLog()
			}()
			if len(resolve) > 0 {
				state.Route = resolve[0](r)
			}
			next.ServeHTTP(buffer, r)
		})
	}
}

func boundedMethod(method string) string {
	switch method {
	case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "CONNECT", "OPTIONS", "TRACE":
		return method
	default:
		return "UNKNOWN"
	}
}

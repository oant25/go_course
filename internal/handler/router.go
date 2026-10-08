package handler

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/go-chi/chi/v5"
	oapimw "github.com/oapi-codegen/nethttp-middleware"

	api "github.com/oant25/go_course/internal/generated"
)

func NewRouter(h *Handler) (http.Handler, error) {
	spec, err := api.GetSwagger()
	if err != nil {
		return nil, fmt.Errorf("load embedded OpenAPI spec: %w", err)
	}
	spec.Servers = nil
	openapi3.SchemaErrorDetailsDisabled = true

	r := chi.NewRouter()
	r.Use(recoverer)
	r.Use(oapimw.OapiRequestValidatorWithOptions(spec, &oapimw.Options{
		Options:              openapi3filter.Options{MultiError: false},
		DoNotValidateServers: true,
		ErrorHandlerWithOpts: func(_ context.Context, err error, w http.ResponseWriter, r *http.Request, opts oapimw.ErrorHandlerOpts) {
			switch opts.StatusCode {
			case http.StatusNotFound, http.StatusMethodNotAllowed:
				http.Error(w, http.StatusText(opts.StatusCode), opts.StatusCode)
			default:
				writeProblem(w, r, problemInvalidRequest, err.Error())
			}
		},
	}))

	return api.HandlerWithOptions(h, api.ChiServerOptions{
		BaseRouter: r,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeProblem(w, r, problemInvalidRequest, err.Error())
		},
	}), nil
}

func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				slog.Error("panic in handler", "panic", rec, "path", r.URL.Path)
				writeProblem(w, r, problemInternal, "Internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

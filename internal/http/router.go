package transport

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/go-chi/chi/v5"
	oapimw "github.com/oapi-codegen/nethttp-middleware"

	api "github.com/oant25/go_course/internal/generated"
	"github.com/oant25/go_course/internal/http/domainerror"
	"github.com/oant25/go_course/internal/http/middleware"
)

func NewRouter(server *Server, logger *slog.Logger, bufferLimit int, providers ...middleware.Providers) (http.Handler, error) {
	spec, err := api.GetSpec()
	if err != nil {
		return nil, fmt.Errorf("load embedded OpenAPI: %w", err)
	}
	spec.Servers = nil
	problems := domainerror.NewResponder()
	router := chi.NewRouter()
	notFound := func(w http.ResponseWriter, r *http.Request) { problems.Write(w, r, domainerror.ErrRouteNotFound) }
	notAllowed := func(w http.ResponseWriter, r *http.Request) {
		methods := allowedMethods(router, routingPath(r))
		if len(methods) > 0 {
			w.Header().Set("Allow", strings.Join(methods, ", "))
		}
		problems.Write(w, r, domainerror.ErrMethodNotAllowed)
	}
	router.Use(middleware.SingleJSONBody(problems))
	router.Use(oapimw.OapiRequestValidatorWithOptions(spec, &oapimw.Options{
		Options: openapi3filter.Options{MultiError: false}, DoNotValidateServers: true,
		ErrorHandlerWithOpts: func(_ context.Context, err error, w http.ResponseWriter, r *http.Request, opts oapimw.ErrorHandlerOpts) {
			switch opts.StatusCode {
			case http.StatusNotFound:
				if len(allowedMethods(router, routingPath(r))) > 0 {
					notAllowed(w, r)
				} else {
					notFound(w, r)
				}
			case http.StatusMethodNotAllowed:
				notAllowed(w, r)
			default:
				problems.WriteInvalidRequest(w, r, err)
			}
		},
	}))
	router.NotFound(notFound)
	router.MethodNotAllowed(notAllowed)
	strict := api.NewStrictHandlerWithOptions(server, nil, api.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  problems.WriteInvalidRequest,
		ResponseErrorHandlerFunc: problems.Write,
	})
	handler := api.HandlerWithOptions(strict, api.ChiServerOptions{BaseRouter: router, ErrorHandlerFunc: problems.WriteInvalidRequest})
	var instrumentation middleware.Providers
	if len(providers) > 0 {
		instrumentation = providers[0]
	}
	return middleware.Observe(handler, logger, bufferLimit, func(r *http.Request) string {
		route := chi.NewRouteContext()
		if router.Match(route, r.Method, routingPath(r)) {
			return route.RoutePattern()
		}
		for _, method := range []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "CONNECT", "OPTIONS", "TRACE"} {
			route = chi.NewRouteContext()
			if router.Match(route, method, routingPath(r)) {
				return route.RoutePattern()
			}
		}
		return "unmatched"
	}, instrumentation)
}

func allowedMethods(router chi.Routes, path string) []string {
	methods := make([]string, 0, 2)
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodConnect, http.MethodOptions, http.MethodTrace} {
		if router.Match(chi.NewRouteContext(), method, path) {
			methods = append(methods, method)
		}
	}
	return methods
}

func routingPath(r *http.Request) string {
	if r.URL.RawPath != "" {
		return r.URL.RawPath
	}
	return r.URL.Path
}

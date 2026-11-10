package domainerror

import (
	"errors"
	"net/http"

	"github.com/oant25/go_course/internal/domain/trip"
	api "github.com/oant25/go_course/internal/generated"
)

var (
	ErrInvalidRequest   = errors.New("request does not match the API contract")
	ErrRouteNotFound    = errors.New("route not found")
	ErrMethodNotAllowed = errors.New("method not allowed")
	ErrInternal         = errors.New("internal server error")
)

func Problem(err error) (int, api.Problem) {
	var validation *trip.ValidationError
	switch {
	case errors.Is(err, trip.ErrInvalidState), errors.Is(err, ErrInternal):
		return problem(http.StatusInternalServerError, "internal_error", "Internal Server Error", "internal-error", "Internal server error")
	case errors.Is(err, trip.ErrTripNotFound):
		return problem(http.StatusNotFound, "trip_not_found", "Trip not found", "trip-not-found", "Trip was not found")
	case errors.Is(err, trip.ErrTripCompleted):
		return problem(http.StatusConflict, "trip_completed", "Trip completed", "trip-completed", "Operation is not allowed for a completed trip")
	case errors.Is(err, trip.ErrDriverBusy):
		return problem(http.StatusConflict, "driver_busy", "Driver busy", "driver-busy", "Driver already has an active trip")
	case errors.As(err, &validation):
		return problem(http.StatusBadRequest, "invalid_request", "Invalid request", "invalid-request", validation.Error())
	case errors.Is(err, ErrInvalidRequest):
		return problem(http.StatusBadRequest, "invalid_request", "Invalid request", "invalid-request", "Request does not match the API contract")
	case errors.Is(err, ErrRouteNotFound):
		return problem(http.StatusNotFound, "route_not_found", "Route not found", "route-not-found", "Route was not found")
	case errors.Is(err, ErrMethodNotAllowed):
		return problem(http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "method-not-allowed", "Method is not allowed for this route")
	default:
		return problem(http.StatusInternalServerError, "internal_error", "Internal Server Error", "internal-error", "Internal server error")
	}
}

func problem(status int32, code, title, slug, detail string) (int, api.Problem) {
	return int(status), api.Problem{Type: "https://tripgo.example/problems/" + slug, Title: title, Status: status, Code: code, Detail: &detail}
}

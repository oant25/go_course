package domainerror_test

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/oant25/go_course/internal/domain/trip"
	"github.com/oant25/go_course/internal/http/domainerror"
)

func TestProblem(t *testing.T) {
	t.Parallel()
	_, corrupt := trip.Restore(trip.State{})
	for _, tc := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"missing", trip.ErrTripNotFound, 404, "trip_not_found"},
		{"completed", trip.ErrTripCompleted, 409, "trip_completed"},
		{"busy", trip.ErrDriverBusy, 409, "driver_busy"},
		{"validation", &trip.ValidationError{Field: "price", Reason: "must be non-negative"}, 400, "invalid_request"},
		{"request", domainerror.ErrInvalidRequest, 400, "invalid_request"},
		{"route", domainerror.ErrRouteNotFound, 404, "route_not_found"},
		{"method", domainerror.ErrMethodNotAllowed, 405, "method_not_allowed"},
		{"corrupt wraps validation", corrupt, 500, internalErrorCode},
		{"internal overrides validation", errors.Join(domainerror.ErrInternal, &trip.ValidationError{Field: "id", Reason: "invalid"}), 500, internalErrorCode},
		{"unknown", errors.New("SQL password=secret-marker"), 500, internalErrorCode},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, err := range []error{tc.err, fmt.Errorf("operation: %w", tc.err)} {
				status, body := domainerror.Problem(err)
				if status != tc.status || int(body.Status) != tc.status || body.Code != tc.code {
					t.Fatalf("problem=%+v status=%d", body, status)
				}
				if body.Detail == nil || body.Title == "" || !strings.HasPrefix(body.Type, "https://tripgo.example/problems/") {
					t.Fatalf("incomplete problem=%+v", body)
				}
				if status == http.StatusInternalServerError && *body.Detail != "Internal server error" {
					t.Fatalf("internal details exposed: %q", *body.Detail)
				}
			}
		})
	}
}

const (
	internalErrorCode = "internal_error"
)

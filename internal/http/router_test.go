package transport_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/oant25/go_course/internal/domain/trip"
	api "github.com/oant25/go_course/internal/generated"
	transport "github.com/oant25/go_course/internal/http"
	"github.com/oant25/go_course/internal/http/health"
)

const validBody = `{"user_id":"10000000-0000-0000-0000-000000000001","driver_id":"20000000-0000-0000-0000-000000000002","start_point":{"latitude":55,"longitude":37},"end_point":{"latitude":59,"longitude":30},"price":100}`

func routerFor(t *testing.T, create *MockcreateHandler, get *MockgetHandler, finish *MockfinishHandler, ready *MockreadyHandler) http.Handler {
	t.Helper()
	router, err := transport.NewRouter(transport.NewServer(create, get, finish, health.New(), ready), slog.New(slog.NewJSONHandler(io.Discard, nil)), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func request(router http.Handler, method, path, body, contentType string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}

func assertProblem(t *testing.T, response *httptest.ResponseRecorder, status int, code, slug string) {
	t.Helper()
	if response.Code != status || response.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}
	var problem api.Problem
	if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	if int(problem.Status) != status || problem.Code != code || problem.Type != "https://tripgo.example/problems/"+slug || problem.Title == "" || problem.Instance == nil {
		t.Fatalf("unexpected problem: %+v", problem)
	}
}

func TestRouterRejectsInvalidInputBeforeHandler(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, method, path, body, contentType string }{
		{"empty", http.MethodPost, tripsPath, "", jsonContentType},
		{"broken", http.MethodPost, tripsPath, "{", jsonContentType},
		{"null", http.MethodPost, tripsPath, "null", jsonContentType},
		{"missing fields", http.MethodPost, tripsPath, "{}", jsonContentType},
		{"two values", http.MethodPost, tripsPath, validBody + " {}", jsonContentType},
		{"unknown field", http.MethodPost, tripsPath, strings.Replace(validBody, `"price":100`, `"secret_marker":1,"price":100`, 1), jsonContentType},
		{"unknown coordinate field", http.MethodPost, tripsPath, strings.Replace(validBody, `"latitude":55`, `"extra":1,"latitude":55`, 1), jsonContentType},
		{"latitude", http.MethodPost, tripsPath, strings.Replace(validBody, `"latitude":55`, `"latitude":91`, 1), jsonContentType},
		{"longitude", http.MethodPost, tripsPath, strings.Replace(validBody, `"longitude":37`, `"longitude":181`, 1), jsonContentType},
		{"negative price", http.MethodPost, tripsPath, strings.Replace(validBody, `"price":100`, `"price":-1`, 1), jsonContentType},
		{"fractional price", http.MethodPost, tripsPath, strings.Replace(validBody, `"price":100`, `"price":1.5`, 1), jsonContentType},
		{"wrong type", http.MethodPost, tripsPath, strings.Replace(validBody, `"price":100`, `"price":"secret_marker"`, 1), jsonContentType},
		{"body UUID", http.MethodPost, tripsPath, strings.Replace(validBody, "10000000-0000-0000-0000-000000000001", "secret_marker", 1), jsonContentType},
		{"path UUID", http.MethodGet, "/api/v1/trips/not-a-uuid", "", ""},
		{"finish UUID", http.MethodPost, "/api/v1/trips/not-a-uuid/finish", "", ""},
		{"content type", http.MethodPost, tripsPath, validBody, "text/plain"},
		{"size limit", http.MethodPost, tripsPath, validBody + strings.Repeat(" ", 1<<20), jsonContentType},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			router := routerFor(t, NewMockcreateHandler(ctrl), NewMockgetHandler(ctrl), NewMockfinishHandler(ctrl), NewMockreadyHandler(ctrl))
			response := request(router, tc.method, tc.path, tc.body, tc.contentType)
			assertProblem(t, response, 400, invalidRequestCode, invalidRequestSlug)
			if strings.Contains(response.Body.String(), "secret_marker") {
				t.Fatal("parser details leaked")
			}
		})
	}
}

func TestRouterWrappedErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		cause      error
		status     int
		code, slug string
	}{
		{"busy", trip.ErrDriverBusy, 409, "driver_busy", "driver-busy"},
		{"completed", trip.ErrTripCompleted, 409, "trip_completed", "trip-completed"},
		{"missing", trip.ErrTripNotFound, 404, "trip_not_found", "trip-not-found"},
		{"repository", errors.New("secret_marker"), 500, internalFailureCode, internalFailureSlug},
		{"corrupt", trip.ErrInvalidState, 500, internalFailureCode, internalFailureSlug},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			get := NewMockgetHandler(ctrl)
			get.EXPECT().GetTrip(gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("wrapped: %w", tc.cause))
			response := request(routerFor(t, nil, get, nil, nil), http.MethodGet, "/api/v1/trips/"+uuid.NewString(), "", "")
			assertProblem(t, response, tc.status, tc.code, tc.slug)
			if strings.Contains(response.Body.String(), "secret_marker") {
				t.Fatal("internal error leaked")
			}
		})
	}
}

func TestRouterHealthAndRouting(t *testing.T) {
	t.Parallel()
	router := routerFor(t, nil, nil, nil, nil)
	response := request(router, http.MethodGet, "/health", "", "")
	if response.Code != 200 || !strings.HasPrefix(response.Header().Get("Content-Type"), jsonContentType) || strings.TrimSpace(response.Body.String()) != `{"status":"ok"}` {
		t.Fatalf("health: %d %s", response.Code, response.Body.String())
	}
	assertProblem(t, request(router, http.MethodGet, "/unknown", "", ""), 404, "route_not_found", "route-not-found")
	response = request(router, http.MethodDelete, tripsPath, "", "")
	assertProblem(t, response, 405, "method_not_allowed", "method-not-allowed")
	if response.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("Allow=%q", response.Header().Get("Allow"))
	}
}

func TestRouterPanicIsRecovered(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	get := NewMockgetHandler(ctrl)
	get.EXPECT().GetTrip(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, api.GetTripRequestObject) (api.GetTripResponseObject, error) {
		panic("secret_marker")
	})
	response := request(routerFor(t, nil, get, nil, nil), http.MethodGet, "/api/v1/trips/"+uuid.NewString(), "", "")
	assertProblem(t, response, 500, internalFailureCode, internalFailureSlug)
	if strings.Contains(response.Body.String(), "secret_marker") {
		t.Fatal("panic leaked")
	}
}

func TestRouterSuccessAndRequestConversion(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	create := NewMockcreateHandler(ctrl)
	id := uuid.New()
	location := "/api/v1/trips/" + id.String()
	create.EXPECT().CreateTrip(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, req api.CreateTripRequestObject) (api.CreateTripResponseObject, error) {
		if ctx == nil || req.Body == nil || req.Body.Price != 100 || req.Body.StartPoint.Latitude != 55 || req.Body.DriverId.String() != "20000000-0000-0000-0000-000000000002" {
			t.Fatalf("incorrect request conversion: %+v", req)
		}
		return api.CreateTrip201JSONResponse{Body: api.Trip{Id: id, Status: api.TripStatus("active")}, Headers: api.CreateTrip201ResponseHeaders{Location: &location}}, nil
	})
	response := request(routerFor(t, create, nil, nil, nil), http.MethodPost, tripsPath, validBody, "application/json; charset=utf-8")
	if response.Code != 201 || response.Header().Get("Location") != location {
		t.Fatalf("create=%d %v %s", response.Code, response.Header(), response.Body.String())
	}
	get := NewMockgetHandler(ctrl)
	get.EXPECT().GetTrip(gomock.Any(), api.GetTripRequestObject{TripId: id}).Return(api.GetTrip200JSONResponse{Id: id}, nil)
	response = request(routerFor(t, nil, get, nil, nil), http.MethodGet, location, "", "")
	if response.Code != 200 {
		t.Fatalf("get=%d %s", response.Code, response.Body.String())
	}
	finish := NewMockfinishHandler(ctrl)
	finish.EXPECT().FinishTrip(gomock.Any(), api.FinishTripRequestObject{TripId: id}).Return(api.FinishTrip200JSONResponse{Id: id, Status: api.TripStatus("completed")}, nil)
	response = request(routerFor(t, nil, nil, finish, nil), http.MethodPost, location+"/finish", "", "")
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"status":"completed"`) {
		t.Fatalf("finish=%d %s", response.Code, response.Body.String())
	}
}

func TestRouterReadinessStatus(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		value  api.ReadyResponseObject
		status int
	}{
		{"ok", api.Ready200JSONResponse{Status: api.Ok}, 200},
		{"unavailable", api.Ready503JSONResponse{Status: api.Unavailable}, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ready := NewMockreadyHandler(gomock.NewController(t))
			ready.EXPECT().Ready(gomock.Any(), api.ReadyRequestObject{}).Return(tc.value, nil)
			response := request(routerFor(t, nil, nil, nil, ready), http.MethodGet, "/ready", "", "")
			if response.Code != tc.status || !strings.HasPrefix(response.Header().Get("Content-Type"), jsonContentType) {
				t.Fatalf("ready=%d %s", response.Code, response.Body.String())
			}
		})
	}
}

const (
	tripsPath = "/api/v1/trips"
)

const (
	jsonContentType     = "application/json"
	invalidRequestCode  = "invalid_request"
	invalidRequestSlug  = "invalid-request"
	internalFailureCode = "internal_error"
	internalFailureSlug = "internal-error"
)

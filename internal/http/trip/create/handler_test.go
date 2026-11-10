package create_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/oant25/go_course/internal/domain/trip"
	api "github.com/oant25/go_course/internal/generated"
	"github.com/oant25/go_course/internal/http/domainerror"
	"github.com/oant25/go_course/internal/http/trip/create"
	"github.com/oant25/go_course/internal/model"
)

func TestCreateTrip(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	usecase := NewMockusecase(ctrl)
	input := model.CreateTripInput{UserID: uuid.New(), DriverID: uuid.New(), Start: trip.Point{Latitude: 55, Longitude: 37}, End: trip.Point{Latitude: 59, Longitude: 30}, Price: 100}
	at := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	value, err := trip.New(uuid.New(), trip.Details{UserID: input.UserID, DriverID: input.DriverID, Start: input.Start, End: input.End, Price: input.Price}, at)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	usecase.EXPECT().Execute(ctx, input).Return(value, nil)
	body := api.CreateTripJSONRequestBody{UserId: input.UserID, DriverId: input.DriverID, StartPoint: api.Coordinates{Latitude: 55, Longitude: 37}, EndPoint: api.Coordinates{Latitude: 59, Longitude: 30}, Price: 100}
	result, err := create.New(usecase).CreateTrip(ctx, api.CreateTripRequestObject{Body: &body})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	if err := result.VisitCreateTripResponse(response); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if response.Code != 201 || response.Header().Get("Location") != "/api/v1/trips/"+value.Snapshot().ID.String() || response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("response=%d headers=%v", response.Code, response.Header())
	}
	if got["id"] != value.Snapshot().ID.String() || got["user_id"] != input.UserID.String() || got["driver_id"] != input.DriverID.String() || got["price"] != float64(100) || got["status"] != "active" || got["started_at"] != at.Format(time.RFC3339) {
		t.Fatalf("body=%v", got)
	}
	for _, key := range []string{"finished_at", "last_position_at"} {
		if value, ok := got[key]; !ok || value != nil {
			t.Fatalf("%s=%v present=%v", key, value, ok)
		}
	}
	for key, want := range map[string]map[string]any{"start_point": {"latitude": float64(55), "longitude": float64(37)}, "end_point": {"latitude": float64(59), "longitude": float64(30)}} {
		point, ok := got[key].(map[string]any)
		if !ok || point["latitude"] != want["latitude"] || point["longitude"] != want["longitude"] {
			t.Fatalf("%s=%v", key, got[key])
		}
	}
}

func TestCreateTripMissingBodyDoesNotExecute(t *testing.T) {
	t.Parallel()
	usecase := NewMockusecase(gomock.NewController(t))
	result, err := create.New(usecase).CreateTrip(context.Background(), api.CreateTripRequestObject{})
	if result != nil || !errors.Is(err, domainerror.ErrInvalidRequest) {
		t.Fatalf("result=%v error=%v", result, err)
	}
}

func TestCreateTripPreservesErrors(t *testing.T) {
	t.Parallel()
	for _, cause := range []error{trip.ErrDriverBusy, &trip.ValidationError{Field: "user_id", Reason: "required"}, errors.New("repository failure")} {
		t.Run(cause.Error(), func(t *testing.T) {
			usecase := NewMockusecase(gomock.NewController(t))
			usecase.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(trip.Trip{}, fmt.Errorf("usecase: %w", cause))
			result, err := create.New(usecase).CreateTrip(context.Background(), api.CreateTripRequestObject{Body: &api.CreateTripJSONRequestBody{}})
			if result != nil || !errors.Is(err, cause) {
				t.Fatalf("result=%v error=%v", result, err)
			}
		})
	}
}

package finish_test

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
	"github.com/oant25/go_course/internal/http/trip/finish"
)

func TestFinishTrip(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	at := time.Date(2026, 11, 10, 12, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	value, err := trip.New(id, trip.Details{UserID: uuid.New(), DriverID: uuid.New(), Price: 100}, at)
	if err != nil {
		t.Fatal(err)
	}
	if err := value.Finish(at.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	ctrl := gomock.NewController(t)
	usecase := NewMockusecase(ctrl)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	usecase.EXPECT().Execute(ctx, id).Return(value, nil)
	result, err := finish.New(usecase).FinishTrip(ctx, api.FinishTripRequestObject{TripId: id})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	if err := result.VisitFinishTripResponse(response); err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 || body["id"] != id.String() || body["status"] != "completed" || body["price"] != float64(100) || body["last_position_at"] != nil {
		t.Fatalf("unexpected response: %d %v", response.Code, body)
	}
	if finished, ok := body["finished_at"]; !ok || (finished != nil) != true {
		t.Fatalf("finished_at=%v present=%v", finished, ok)
	}
	if body["started_at"] != at.UTC().Format(time.RFC3339) {
		t.Fatalf("started_at=%v", body["started_at"])
	}
}

func TestFinishTripErrorPreservesCause(t *testing.T) {
	t.Parallel()
	for _, cause := range []error{trip.ErrTripNotFound, trip.ErrTripCompleted, errors.New("repository failure")} {
		t.Run(cause.Error(), func(t *testing.T) {
			ctrl := gomock.NewController(t)
			usecase := NewMockusecase(ctrl)
			id := uuid.New()
			usecase.EXPECT().Execute(gomock.Any(), id).Return(trip.Trip{}, fmt.Errorf("repository: %w", cause))
			result, err := finish.New(usecase).FinishTrip(context.Background(), api.FinishTripRequestObject{TripId: id})
			if result != nil || !errors.Is(err, cause) {
				t.Fatalf("result=%v error=%v", result, err)
			}
		})
	}
}

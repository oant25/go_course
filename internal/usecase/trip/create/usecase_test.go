package create_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/oant25/go_course/internal/domain/trip"
	"github.com/oant25/go_course/internal/model"
	"github.com/oant25/go_course/internal/usecase/trip/create"
)

func validInput() model.CreateTripInput {
	return model.CreateTripInput{UserID: uuid.MustParse("10000000-0000-0000-0000-000000000001"), DriverID: uuid.MustParse("20000000-0000-0000-0000-000000000002"), Start: trip.Point{Latitude: 55.75, Longitude: 37.61}, End: trip.Point{Latitude: 59.93, Longitude: 30.31}, Price: 1450}
}

func TestExecute(t *testing.T) {
	t.Parallel()
	failure := errors.New("storage unavailable")
	for _, tc := range []struct {
		name, stage string
		wantErr     error
	}{
		{"success", "", nil},
		{"begin error", beginStage, failure},
		{"driver busy", insertStage, trip.ErrDriverBusy},
		{"insert error", insertStage, failure},
		{"history error", "history", failure},
		{"commit error", "commit", failure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			repo := NewMocktripRepository(ctrl)
			tx := NewMocktxManager(ctrl)
			events := NewMockrecorder(ctrl)
			clock := NewMockclock(ctrl)
			ids := NewMockidGenerator(ctrl)
			ctx := context.Background()
			type txKey struct{}
			txCtx := context.WithValue(ctx, txKey{}, true)
			at := time.Date(2026, 11, 10, 12, 30, 0, 123456000, time.FixedZone("MSK", 10800))
			id := uuid.MustParse("30000000-0000-0000-0000-000000000003")
			in := validInput()
			expected, err := trip.New(id, trip.Details{UserID: in.UserID, DriverID: in.DriverID, Start: in.Start, End: in.End, Price: in.Price}, at)
			if err != nil {
				t.Fatal(err)
			}
			gomock.InOrder(clock.EXPECT().Now().Return(at), ids.EXPECT().New().Return(id))
			var insertCall *gomock.Call
			if tc.stage != beginStage {
				insertCall = repo.EXPECT().Create(txCtx, gomock.Any()).DoAndReturn(func(gotCtx context.Context, value trip.Trip) (trip.Trip, error) {
					if gotCtx != txCtx {
						t.Fatal("repository did not receive transaction context")
					}
					if !reflect.DeepEqual(value.Snapshot(), expected.Snapshot()) {
						t.Fatalf("inserted trip = %+v, want %+v", value.Snapshot(), expected.Snapshot())
					}
					if tc.stage == insertStage {
						return trip.Trip{}, fmt.Errorf("database: %w", tc.wantErr)
					}
					return value, nil
				})
				if tc.stage != insertStage {
					history := repo.EXPECT().AddStatusChange(txCtx, id, (*trip.Status)(nil), trip.StatusActive, at.UTC())
					if tc.stage == "history" {
						history.Return(tc.wantErr)
					} else {
						history.Return(nil)
					}
					gomock.InOrder(insertCall, history)
				}
			}
			committed := false
			tx.EXPECT().Do(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, fn func(context.Context) error) error {
				if tc.stage == beginStage {
					return tc.wantErr
				}
				err := fn(txCtx)
				if tc.stage == "commit" {
					if err != nil {
						t.Fatalf("callback unexpectedly failed before commit: %v", err)
					}
					return tc.wantErr
				}
				committed = err == nil
				return err
			})
			if tc.wantErr == nil {
				events.EXPECT().TripCreated(ctx, id).Do(func(context.Context, uuid.UUID) {
					if !committed {
						t.Fatal("business event emitted before commit")
					}
				})
			}
			got, err := create.New(repo, tx, clock, ids, events).Execute(ctx, in)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				if err == tc.wantErr {
					t.Fatal("error was not wrapped with operation context")
				}
				if !reflect.DeepEqual(got, trip.Trip{}) {
					t.Fatal("failed transaction returned a trip")
				}
			} else if !reflect.DeepEqual(got.Snapshot(), expected.Snapshot()) {
				t.Fatal("wrong created result")
			}
		})
	}
}

func TestInvalidInputHasNoSideEffects(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, field string
		change      func(*model.CreateTripInput)
	}{
		{"user", "user_id", func(in *model.CreateTripInput) { in.UserID = uuid.Nil }},
		{"driver", "driver_id", func(in *model.CreateTripInput) { in.DriverID = uuid.Nil }},
		{"price", "price", func(in *model.CreateTripInput) { in.Price = -1 }},
		{"latitude", "start_point.latitude", func(in *model.CreateTripInput) { in.Start.Latitude = 91 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			u := create.New(NewMocktripRepository(ctrl), NewMocktxManager(ctrl), NewMockclock(ctrl), NewMockidGenerator(ctrl), NewMockrecorder(ctrl))
			in := validInput()
			tc.change(&in)
			got, err := u.Execute(context.Background(), in)
			var validation *trip.ValidationError
			if !errors.As(err, &validation) || validation.Field != tc.field {
				t.Fatalf("validation: %v", err)
			}
			if !reflect.DeepEqual(got, trip.Trip{}) {
				t.Fatal("invalid input returned a trip")
			}
		})
	}
}

func TestInvalidGeneratedIDDoesNotOpenTransaction(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	events := NewMockrecorder(ctrl)
	clock := NewMockclock(ctrl)
	ids := NewMockidGenerator(ctrl)
	clock.EXPECT().Now().Return(time.Time{})
	ids.EXPECT().New().Return(uuid.Nil)
	_, err := create.New(NewMocktripRepository(ctrl), NewMocktxManager(ctrl), clock, ids, events).Execute(context.Background(), validInput())
	var validation *trip.ValidationError
	if !errors.As(err, &validation) || validation.Field != "id" {
		t.Fatalf("unexpected error: %v", err)
	}
}

const (
	beginStage  = "begin"
	insertStage = "insert"
)

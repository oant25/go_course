package get_test

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
	"github.com/oant25/go_course/internal/usecase/trip/get"
)

func TestExecute(t *testing.T) {
	t.Parallel()
	id := uuid.MustParse("30000000-0000-0000-0000-000000000003")
	value, err := trip.New(id, trip.Details{UserID: uuid.New(), DriverID: uuid.New()}, time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("database unavailable")
	for _, tc := range []struct {
		name    string
		wantErr error
	}{
		{"success", nil}, {"not found", trip.ErrTripNotFound}, {"repository failure", failure}, {"corrupt stored data", trip.ErrInvalidState},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMocktripRepository(ctrl)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var repoErr error
			if tc.wantErr != nil {
				repoErr = fmt.Errorf("select trip: %w", tc.wantErr)
			}
			repo.EXPECT().GetByID(ctx, id).Return(value, repoErr)
			got, err := get.New(repo).Execute(ctx, id)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				if err == repoErr {
					t.Fatal("operation context not added")
				}
				if !reflect.DeepEqual(got, trip.Trip{}) {
					t.Fatal("error leaked a partial result")
				}
			} else if !reflect.DeepEqual(got.Snapshot(), value.Snapshot()) {
				t.Fatal("wrong trip returned")
			}
		})
	}
}

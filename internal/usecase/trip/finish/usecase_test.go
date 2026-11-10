package finish_test

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
	"github.com/oant25/go_course/internal/usecase/trip/finish"
)

func TestExecute(t *testing.T) {
	t.Parallel()
	failure := errors.New("database unavailable")
	for _, tc := range []struct {
		name, stage string
		wantErr     error
	}{
		{"success", "", nil},
		{"begin error", beginStage, failure},
		{"not found", loadStage, trip.ErrTripNotFound},
		{"read error", loadStage, failure},
		{"already completed", completedStage, trip.ErrTripCompleted},
		{"corrupt state", corruptStage, trip.ErrInvalidState},
		{"save error", saveStage, failure},
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
			ctx := context.Background()
			type txKey struct{}
			txCtx := context.WithValue(ctx, txKey{}, true)
			id := uuid.MustParse("30000000-0000-0000-0000-000000000003")
			at := time.Date(2026, 11, 10, 12, 0, 0, 0, time.FixedZone("MSK", 10800))
			original, err := trip.New(id, trip.Details{UserID: uuid.New(), DriverID: uuid.New(), Price: 1450}, at.Add(-time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			expected := original
			if err := expected.Finish(at); err != nil {
				t.Fatal(err)
			}
			loaded := original
			if tc.stage == completedStage {
				loaded = expected
			}
			if tc.stage == corruptStage {
				loaded = trip.Trip{}
			}
			if tc.stage != beginStage {
				var readErr error
				if tc.stage == loadStage {
					readErr = fmt.Errorf("select: %w", tc.wantErr)
				}
				read := repo.EXPECT().GetByIDForUpdate(txCtx, id).Return(loaded, readErr)
				if tc.stage != loadStage {
					now := clock.EXPECT().Now().Return(at)
					gomock.InOrder(read, now)
					if tc.stage != completedStage && tc.stage != corruptStage {
						save := repo.EXPECT().SaveFinished(txCtx, gomock.Any()).DoAndReturn(func(gotCtx context.Context, value trip.Trip) (trip.Trip, error) {
							if gotCtx != txCtx {
								t.Fatal("wrong transaction context")
							}
							if !reflect.DeepEqual(value.Snapshot(), expected.Snapshot()) {
								t.Fatalf("saved trip=%+v, want %+v", value.Snapshot(), expected.Snapshot())
							}
							if tc.stage == saveStage {
								return trip.Trip{}, tc.wantErr
							}
							return value, nil
						})
						gomock.InOrder(now, save)
						if tc.stage != saveStage {
							from := trip.StatusActive
							history := repo.EXPECT().AddStatusChange(txCtx, id, &from, trip.StatusCompleted, at.UTC())
							if tc.stage == "history" {
								history.Return(tc.wantErr)
							} else {
								history.Return(nil)
							}
							gomock.InOrder(save, history)
						}
					}
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
						t.Fatalf("callback failed before commit: %v", err)
					}
					return tc.wantErr
				}
				committed = err == nil
				return err
			})
			if tc.wantErr == nil {
				events.EXPECT().TripCompleted(ctx, id).Do(func(context.Context, uuid.UUID) {
					if !committed {
						t.Fatal("business event emitted before commit")
					}
				})
			}
			got, err := finish.New(repo, tx, clock, events).Execute(ctx, id)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				if err == tc.wantErr {
					t.Fatal("operation context not added")
				}
				if !reflect.DeepEqual(got, trip.Trip{}) {
					t.Fatal("failed transaction returned a result")
				}
			} else if !reflect.DeepEqual(got.Snapshot(), expected.Snapshot()) {
				t.Fatal("wrong completed result")
			}
			if original.Snapshot().Status != trip.StatusActive {
				t.Fatal("usecase mutated repository-owned trip")
			}
		})
	}
}

const (
	beginStage     = "begin"
	loadStage      = "load"
	completedStage = "completed"
	corruptStage   = "corrupt"
	saveStage      = "save"
)

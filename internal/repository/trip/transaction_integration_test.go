//go:build integration

package trip_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/oant25/go_course/internal/domain/trip"
	"github.com/oant25/go_course/internal/infra/postgres/transaction"
	triprepo "github.com/oant25/go_course/internal/repository/trip"
)

func TestNestedTransactionRejectedIntegration(t *testing.T) {
	pool := database(t)
	repo := triprepo.New(pool, testTimeout)
	manager := transaction.New(pool, testTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	in := input()
	value, err := trip.New(uuid.New(), trip.Details{UserID: in.UserID, DriverID: in.DriverID, Start: in.Start, End: in.End, Price: in.Price}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	called := false
	err = manager.Do(ctx, func(txCtx context.Context) error {
		if _, err := repo.Create(txCtx, value); err != nil {
			return err
		}
		return manager.Do(txCtx, func(context.Context) error { called = true; return nil })
	})
	if !errors.Is(err, transaction.ErrNestedTransaction) || called {
		t.Fatalf("nested error=%v callback=%v", err, called)
	}
	if _, err := repo.GetByID(ctx, value.Snapshot().ID); !errors.Is(err, trip.ErrTripNotFound) {
		t.Fatalf("outer insert was not rolled back: %v", err)
	}
	if _, ok := transaction.FromContext(ctx); ok {
		t.Fatal("transaction leaked to parent context")
	}
}

func TestTransactionCleanupIntegration(t *testing.T) {
	for _, scenario := range []string{panicScenario, cancelCallback, "cancel_commit"} {
		t.Run(scenario, func(t *testing.T) {
			pool := database(t)
			repo := triprepo.New(pool, testTimeout)
			manager := transaction.New(pool, testTimeout)
			outer, done := context.WithTimeout(context.Background(), testTimeout)
			defer done()
			ctx, cancel := context.WithCancel(outer)
			defer cancel()
			in := input()
			value, err := trip.New(uuid.New(), trip.Details{UserID: in.UserID, DriverID: in.DriverID}, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			failure := errors.New("transaction aborted")
			var panicked any
			func() {
				defer func() { panicked = recover() }()
				err = manager.Do(ctx, func(txCtx context.Context) error {
					if _, err := repo.Create(txCtx, value); err != nil {
						return err
					}
					if scenario == panicScenario {
						panic(failure)
					}
					cancel()
					if scenario == cancelCallback {
						return failure
					}
					return nil
				})
			}()
			if scenario == panicScenario {
				if panicked != failure {
					t.Fatalf("panic=%v, want original value", panicked)
				}
			} else {
				if panicked != nil {
					t.Fatalf("unexpected panic=%v", panicked)
				}
				if err == nil {
					t.Fatal("cancelled transaction succeeded")
				}
				if scenario == cancelCallback && !errors.Is(err, failure) {
					t.Fatalf("callback cause lost: %v", err)
				}
			}
			if _, err := repo.GetByID(outer, value.Snapshot().ID); !errors.Is(err, trip.ErrTripNotFound) {
				t.Fatalf("insert survived cleanup: %v", err)
			}
			if err := pool.Ping(outer); err != nil {
				t.Fatalf("pool unusable after cleanup: %v", err)
			}
		})
	}
}

func TestConcurrentCreateIntegration(t *testing.T) {
	pool := database(t)
	trips := newService(pool)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	in := input()
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() { defer workers.Done(); <-start; _, err := trips.Create(ctx, in); results <- err }()
	}
	close(start)
	workers.Wait()
	close(results)
	success, busy := 0, 0
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, trip.ErrDriverBusy):
			busy++
		default:
			t.Fatalf("create error=%v", err)
		}
	}
	if success != 1 || busy != 1 {
		t.Fatalf("success=%d busy=%d, want 1/1", success, busy)
	}
	var tripsCount, historyCount int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM trips WHERE driver_id=$1", in.DriverID).Scan(&tripsCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM trip_status_history h JOIN trips t ON t.id=h.trip_id WHERE t.driver_id=$1", in.DriverID).Scan(&historyCount); err != nil {
		t.Fatal(err)
	}
	if tripsCount != 1 || historyCount != 1 {
		t.Fatalf("trips=%d history=%d, want 1/1", tripsCount, historyCount)
	}
}

const (
	cancelCallback = "cancel_callback"
)

const (
	panicScenario = "panic"
)
